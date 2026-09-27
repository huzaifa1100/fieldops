package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/nwasiq/fieldops/backend/internal/models"

	"github.com/nwasiq/fieldops/backend/internal/services"
)

// rule: §5.1 — the daily report counts visits by day of scheduled start and sums clocked hours
func TestReport_Daily(t *testing.T) {
	s := newTestServer(t)
	admin := s.createUser("ada.admin@example.com", models.RoleAdmin, "")
	tom := s.createUser("tom.field@example.com", models.RoleTechnician, "")
	ida := s.createUser("ida.inactive@example.com", models.RoleTechnician, "")
	site := s.createSite("Depot")
	adminToken := s.login(admin.Email)

	tomVisit := s.createVisit(site.ID, &tom.ID, "2026-07-14T08:00:00Z", 120)
	idaVisit := s.createVisit(site.ID, &ida.ID, "2026-07-15T10:00:00Z", 60)
	cancelled := s.createVisit(site.ID, &tom.ID, "2026-07-15T11:00:00Z", 60)
	s.createVisit(site.ID, nil, "2026-07-15T12:00:00Z", 60)

	clock := func(visitID uint, email, in, out string) {
		token := s.login(email)
		at, _ := time.Parse(time.RFC3339, in)
		s.Visits.SetNow(func() time.Time { return at })
		s.decode(s.do(http.MethodPost, "/api/visits/"+itoa(visitID)+"/clock-in", token, nil), http.StatusOK, nil)
		at, _ = time.Parse(time.RFC3339, out)
		s.Visits.SetNow(func() time.Time { return at })
		s.decode(s.do(http.MethodPost, "/api/visits/"+itoa(visitID)+"/clock-out", token, nil), http.StatusOK, nil)
	}
	clock(tomVisit.ID, tom.Email, "2026-07-14T08:00:00Z", "2026-07-14T09:30:00Z")
	s.decode(s.do(http.MethodPost, "/api/visits/"+itoa(idaVisit.ID)+"/clock-in", s.login(ida.Email), nil), http.StatusOK, nil)
	s.decode(s.do(http.MethodPost, "/api/visits/"+itoa(cancelled.ID)+"/cancel", adminToken, nil), http.StatusOK, nil)

	s.decode(s.do(http.MethodPut, "/api/users/"+itoa(ida.ID), adminToken, map[string]any{
		"email": ida.Email, "first_name": "Ida", "last_name": "Inactive", "role": models.RoleTechnician, "is_active": false,
	}), http.StatusOK, nil)

	var report dailyReportView
	s.decode(s.do(http.MethodGet, "/api/reports/daily?date=2026-07-15", adminToken, nil), http.StatusOK, &report)
	if report.Date != "2026-07-15" || report.Scheduled != 3 || report.Completed != 0 || report.Cancelled != 1 {
		t.Errorf("15 July totals = %+v", report)
	}
	byName := map[string]technicianDayView{}
	for _, line := range report.Technicians {
		byName[line.Name] = line
	}
	if len(byName) != 2 {
		t.Fatalf("technicians = %+v, want Ida and Tom", report.Technicians)
	}
	if i := byName["Ida Inactive"]; i.Visits != 1 || i.Completed != 0 || i.HoursClocked != 0 {
		t.Errorf("deactivated Ida = %+v, want 1 visit, open clock-in counts no hours", i)
	}
	if tm := byName["Tom Field"]; tm.Visits != 1 || tm.Completed != 0 || tm.HoursClocked != 0 {
		t.Errorf("Tom on the 15th = %+v, want the cancelled visit only", tm)
	}

	s.decode(s.do(http.MethodGet, "/api/reports/daily?date=2026-07-14", adminToken, nil), http.StatusOK, &report)
	if report.Scheduled != 1 || report.Completed != 1 || len(report.Technicians) != 1 || report.Technicians[0].HoursClocked != 1.5 {
		t.Errorf("14 July = %+v, want Tom's single 1.5 h visit", report)
	}
	s.expectError(s.do(http.MethodGet, "/api/reports/daily", adminToken, nil), http.StatusBadRequest)
	s.expectError(s.do(http.MethodGet, "/api/reports/daily?date=15-07-2026", adminToken, nil), http.StatusBadRequest)
}

// rule: §3.3 — a visit whose scheduled start is on Europe/London day D belongs to
// day D in every report, whatever the server's timezone
func TestReport_DayBoundaryIsEuropeLondon(t *testing.T) {
	const day = "2026-09-22" // BST: Europe/London is UTC+1 on this day

	// SQLite compares timestamps as text, so all three zones give the same
	// result here. On Postgres they would differ. The loop still checks that
	// the report ignores the server's timezone.
	zones := []struct {
		name string
		loc  *time.Location
	}{
		{"utc", time.UTC},
		{"east_of_london", time.FixedZone("TEST+9", 9*60*60)},
		{"west_of_london", time.FixedZone("TEST-7", -7*60*60)},
	}

	for _, zone := range zones {
		t.Run(zone.name, func(t *testing.T) {
			orig := time.Local
			t.Cleanup(func() { time.Local = orig })
			time.Local = zone.loc // the report must not care what this is (§5, CONVENTIONS)

			s := newTestServer(t)
			admin := s.createUser("ada.admin@example.com", models.RoleAdmin, "")
			tech := s.createUser("tom.field@example.com", models.RoleTechnician, "")
			site := s.createSite("Depot")
			token := s.login(admin.Email)

			at := func(hour, minute int) string {
				t.Helper()
				instant, err := services.UKClock(day, hour, minute)
				if err != nil {
					t.Fatalf("UKClock(%s %02d:%02d): %v", day, hour, minute, err)
				}
				return instant.Format(time.RFC3339)
			}

			s.createVisit(site.ID, &tech.ID, at(0, 30), 60)  // early morning
			s.createVisit(site.ID, &tech.ID, at(12, 0), 60)  // midday control
			s.createVisit(site.ID, &tech.ID, at(22, 30), 60) // late evening

			var report dailyReportView
			s.decode(s.do(http.MethodGet, "/api/reports/daily?date="+day, token, nil), http.StatusOK, &report)
			if report.Scheduled != 3 {
				t.Errorf("server in %s: %s scheduled = %d, want 3 — visits at 00:30, 12:00 and 22:30 "+
					"Europe/London all belong to %s whatever zone the server runs in (§3.3)",
					zone.name, day, report.Scheduled, day)
			}

			// a displaced visit is wrong twice: missing from its day, present next door
			for _, neighbour := range []string{"2026-09-21", "2026-09-23"} {
				var spill dailyReportView
				s.decode(s.do(http.MethodGet, "/api/reports/daily?date="+neighbour, token, nil), http.StatusOK, &spill)
				if spill.Scheduled != 0 {
					t.Errorf("server in %s: %s scheduled = %d, want 0 — nothing was booked on %s, so a count "+
						"here means a %s visit leaked across the day boundary (§3.3)",
						zone.name, neighbour, spill.Scheduled, neighbour, day)
				}

			}
		})
	}
}

// rule: §3.3 — day membership follows the Europe/London calendar day, including the
// 25-hour day on which British Summer Time ends
func TestReport_ClockChangeDayIsTwentyFiveHours(t *testing.T) {
	const day = "2026-10-25" // BST ends: this Europe/London day runs 25 hours

	orig := time.Local
	t.Cleanup(func() { time.Local = orig })
	time.Local = time.UTC // pinned so this test isolates the 24-hour assumption

	s := newTestServer(t)
	admin := s.createUser("ada.admin@example.com", models.RoleAdmin, "")
	tech := s.createUser("tom.field@example.com", models.RoleTechnician, "")
	site := s.createSite("Depot")
	token := s.login(admin.Email)

	for _, hm := range []struct{ hour, minute int }{{0, 30}, {23, 30}} {
		instant, err := services.UKClock(day, hm.hour, hm.minute)
		if err != nil {
			t.Fatalf("UKClock(%s %02d:%02d): %v", day, hm.hour, hm.minute, err)
		}
		s.createVisit(site.ID, &tech.ID, instant.Format(time.RFC3339), 30)
	}

	var report dailyReportView
	s.decode(s.do(http.MethodGet, "/api/reports/daily?date="+day, token, nil), http.StatusOK, &report)
	if report.Scheduled != 2 {
		t.Errorf("%s scheduled = %d, want 2 — BST ends on this day, so the Europe/London day runs 25 hours "+
			"from 2026-10-24T23:00Z to 2026-10-26T00:00Z and both the 00:30 and 23:30 visits belong to it. "+
			"A fixed 24-hour window drops one of them (§3.3)", day, report.Scheduled)
	}
}

// rule: §5.2 — a technician with visits on the day appears in the report even if they
// were deactivated or soft-deleted afterwards
func TestReport_IncludesSoftDeletedTechnician(t *testing.T) {
	const day = "2026-09-22"

	orig := time.Local
	t.Cleanup(func() { time.Local = orig })
	time.Local = time.UTC // pinned so a day-window bug cannot contaminate this test

	s := newTestServer(t)
	admin := s.createUser("ada.admin@example.com", models.RoleAdmin, "")
	dana := s.createUser("dana.departed@example.com", models.RoleTechnician, "")
	site := s.createSite("Depot")
	adminToken := s.login(admin.Email)
	danaToken := s.login(dana.Email)

	start, err := services.UKClock(day, 9, 0)
	if err != nil {
		t.Fatal(err)
	}
	visit := s.createVisit(site.ID, &dana.ID, start.Format(time.RFC3339), 120)

	// a full 09:00–10:30 pair, so there are real hours to lose
	s.Visits.SetNow(func() time.Time { return start })
	s.decode(s.do(http.MethodPost, "/api/visits/"+itoa(visit.ID)+"/clock-in", danaToken, nil), http.StatusOK, nil)
	s.Visits.SetNow(func() time.Time { return start.Add(90 * time.Minute) })
	s.decode(s.do(http.MethodPost, "/api/visits/"+itoa(visit.ID)+"/clock-out", danaToken, nil), http.StatusOK, nil)

	var before dailyReportView
	s.decode(s.do(http.MethodGet, "/api/reports/daily?date="+day, adminToken, nil), http.StatusOK, &before)
	if len(before.Technicians) != 1 {
		t.Fatalf("precondition: technicians = %+v, want Dana listed while she is still live", before.Technicians)
	}

	// she leaves. Soft-delete, NOT is_active=false: deactivation leaves deleted_at NULL,
	// so a default-scoped preload still resolves her and the bug stays hidden.
	s.decode(s.do(http.MethodDelete, "/api/users/"+itoa(dana.ID), adminToken, nil), http.StatusOK, nil)

	var after dailyReportView
	s.decode(s.do(http.MethodGet, "/api/reports/daily?date="+day, adminToken, nil), http.StatusOK, &after)

	if after.Scheduled != before.Scheduled || after.Completed != before.Completed {
		t.Errorf("day totals moved when the technician was soft-deleted: scheduled %d->%d, completed %d->%d, "+
			"want unchanged (§5.2)", before.Scheduled, after.Scheduled, before.Completed, after.Completed)
	}
	if len(after.Technicians) != 1 {
		t.Fatalf("technicians after soft-delete = %+v, want Dana Departed still listed — a departed "+
			"technician's work stays on the report for the day she did it (§5.2)", after.Technicians)
	}
	line := after.Technicians[0]
	if line.Name != "Dana Departed" {
		t.Errorf("technician name = %q, want %q — attribution must survive soft-delete (§1.2, §5.2)",
			line.Name, "Dana Departed")
	}
	if line.Visits != 1 || line.Completed != 1 || line.HoursClocked != 1.5 {
		t.Errorf("Dana's line = %+v, want 1 visit, 1 completed, 1.5 hours from the 09:00–10:30 pair (§5.3)", line)
	}
}

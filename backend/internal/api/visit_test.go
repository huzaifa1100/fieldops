package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nwasiq/fieldops/backend/internal/models"
	"github.com/nwasiq/fieldops/backend/internal/services"
)

type visitList struct {
	Visits     []visitView `json:"visits"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// rule: §3.2 — from and to are required and must be well-formed Europe/London days
func TestVisits_FromToRequired(t *testing.T) {
	s := newTestServer(t)
	dispatcher := s.createUser("dev.dispatcher@example.com", models.RoleDispatcher, "")
	token := s.login(dispatcher.Email)
	for _, query := range []string{"", "?from=2026-07-14", "?to=2026-07-14", "?from=14/07/2026&to=2026-07-14", "?from=2026-07-16&to=2026-07-14", "?from=2026-07-14&to=2026-07-14&status=done"} {
		if got := s.expectError(s.do(http.MethodGet, "/api/visits"+query, token, nil), http.StatusBadRequest); got == "" {
			t.Errorf("query %q: no error message", query)
		}
	}
	s.decode(s.do(http.MethodGet, "/api/visits?from=2026-07-14&to=2026-07-14", token, nil), http.StatusOK, nil)
}

// rule: §3.3 — a 23:30Z start in BST belongs to the next Europe/London day; in GMT it does not
func TestVisits_UKDayMembershipAtBoundary(t *testing.T) {
	s := newTestServer(t)
	dispatcher := s.createUser("dev.dispatcher@example.com", models.RoleDispatcher, "")
	token := s.login(dispatcher.Email)
	site := s.createSite("Depot")
	lateBST := s.createVisit(site.ID, nil, "2026-07-14T23:30:00Z", 60)   // 00:30 BST on the 15th
	earlyBST := s.createVisit(site.ID, nil, "2026-07-15T00:15:00Z", 60)  // 01:15 BST on the 15th
	middayBST := s.createVisit(site.ID, nil, "2026-07-14T11:00:00Z", 60) // the 14th
	lateGMT := s.createVisit(site.ID, nil, "2026-01-14T23:30:00Z", 60)   // still the 14th in January

	ids := func(day string) map[uint]bool {
		var list visitList
		s.decode(s.do(http.MethodGet, "/api/visits?from="+day+"&to="+day, token, nil), http.StatusOK, &list)
		got := map[uint]bool{}
		for _, v := range list.Visits {
			got[v.ID] = true
		}
		return got
	}
	july14, july15, jan14, jan15 := ids("2026-07-14"), ids("2026-07-15"), ids("2026-01-14"), ids("2026-01-15")
	if july14[lateBST.ID] || !july15[lateBST.ID] {
		t.Errorf("23:30Z in July: on 14th=%v on 15th=%v, want only the 15th", july14[lateBST.ID], july15[lateBST.ID])
	}
	if !july15[earlyBST.ID] || july14[earlyBST.ID] {
		t.Errorf("00:15Z on the 15th in July listed on the wrong day")
	}
	if !july14[middayBST.ID] || len(july14) != 1 {
		t.Errorf("14 July list = %v, want only the midday visit", july14)
	}
	if !jan14[lateGMT.ID] || jan15[lateGMT.ID] {
		t.Errorf("23:30Z in January: on 14th=%v on 15th=%v, want only the 14th", jan14[lateGMT.ID], jan15[lateGMT.ID])
	}

	var span visitList
	s.decode(s.do(http.MethodGet, "/api/visits?from=2026-07-14&to=2026-07-15", token, nil), http.StatusOK, &span)
	if span.Total != 3 {
		t.Errorf("two-day span total = %d, want 3", span.Total)
	}
	for _, v := range span.Visits {
		if v.ScheduledStart[len(v.ScheduledStart)-1] != 'Z' {
			t.Errorf("scheduled_start %q is not UTC RFC3339", v.ScheduledStart)
		}
	}
}

// rule: §3.2 — page_size is clamped to 100 and the effective value echoed
func TestVisits_PageSizeClampAndEcho(t *testing.T) {
	s := newTestServer(t)
	dispatcher := s.createUser("dev.dispatcher@example.com", models.RoleDispatcher, "")
	token := s.login(dispatcher.Email)
	site := s.createSite("Depot")
	start, _ := time.Parse(time.RFC3339, "2026-07-14T06:00:00Z")
	for i := 0; i < 105; i++ {
		s.createVisit(site.ID, nil, start.Add(time.Duration(i)*5*time.Minute).Format(time.RFC3339), 5)
	}
	var list visitList
	s.decode(s.do(http.MethodGet, "/api/visits?from=2026-07-14&to=2026-07-14&page_size=1000", token, nil), http.StatusOK, &list)
	if list.PageSize != services.MaxPageSize || len(list.Visits) != 100 || list.Total != 105 || list.TotalPages != 2 || list.Page != 1 {
		t.Errorf("page 1 = size %d rows %d total %d pages %d page %d", list.PageSize, len(list.Visits), list.Total, list.TotalPages, list.Page)
	}
	s.decode(s.do(http.MethodGet, "/api/visits?from=2026-07-14&to=2026-07-14&page_size=1000&page=2", token, nil), http.StatusOK, &list)
	if len(list.Visits) != 5 || list.Page != 2 {
		t.Errorf("page 2 = %d rows, page %d", len(list.Visits), list.Page)
	}
	s.decode(s.do(http.MethodGet, "/api/visits?from=2026-07-14&to=2026-07-14", token, nil), http.StatusOK, &list)
	if list.PageSize != services.DefaultPageSize || len(list.Visits) != 20 {
		t.Errorf("default page = size %d rows %d", list.PageSize, len(list.Visits))
	}
}

// rule: §1.1 — a technician sees only their own visits, whatever filter they send
func TestVisits_TechnicianSeesOwnOnly(t *testing.T) {
	s := newTestServer(t)
	tom := s.createUser("tom.field@example.com", models.RoleTechnician, "")
	tara := s.createUser("tara.wrench@example.com", models.RoleTechnician, "")
	dispatcher := s.createUser("dev.dispatcher@example.com", models.RoleDispatcher, "")
	site := s.createSite("Depot")
	mine := s.createVisit(site.ID, &tom.ID, "2026-07-14T08:00:00Z", 60)
	theirs := s.createVisit(site.ID, &tara.ID, "2026-07-14T09:00:00Z", 60)
	s.createVisit(site.ID, nil, "2026-07-14T10:00:00Z", 60)
	tomToken := s.login(tom.Email)

	var list visitList
	s.decode(s.do(http.MethodGet, "/api/visits?from=2026-07-14&to=2026-07-14&technician_id="+itoa(tara.ID), tomToken, nil), http.StatusOK, &list)
	if list.Total != 1 || list.Visits[0].ID != mine.ID {
		t.Errorf("technician list = %+v, want only their own visit", list)
	}
	s.decode(s.do(http.MethodGet, "/api/visits/"+itoa(mine.ID), tomToken, nil), http.StatusOK, nil)
	s.expectError(s.do(http.MethodGet, "/api/visits/"+itoa(theirs.ID), tomToken, nil), http.StatusForbidden)
	s.expectError(s.do(http.MethodPost, "/api/visits/"+itoa(theirs.ID)+"/clock-in", tomToken, nil), http.StatusForbidden)

	var all visitList
	s.decode(s.do(http.MethodGet, "/api/visits?from=2026-07-14&to=2026-07-14", s.login(dispatcher.Email), nil), http.StatusOK, &all)
	if all.Total != 3 {
		t.Errorf("dispatcher sees %d visits, want 3", all.Total)
	}
	s.decode(s.do(http.MethodGet, "/api/visits?from=2026-07-14&to=2026-07-14&technician_id="+itoa(tara.ID), s.login(dispatcher.Email), nil), http.StatusOK, &all)
	if all.Total != 1 || all.Visits[0].Technician == nil || all.Visits[0].Technician.FirstName != "Tara" {
		t.Errorf("filtered list = %+v", all)
	}
}

// rule: §4.2 — clock-in moves to in_progress, clock-out to completed; out-without-in and a second in are 409
func TestVisits_ClockStateMachine(t *testing.T) {
	s := newTestServer(t)
	tom := s.createUser("tom.field@example.com", models.RoleTechnician, "")
	site := s.createSite("Depot")
	visit := s.createVisit(site.ID, &tom.ID, "2026-07-14T08:00:00Z", 60)
	token := s.login(tom.Email)
	path := "/api/visits/" + itoa(visit.ID)

	clockIn, _ := time.Parse(time.RFC3339, "2026-07-14T08:03:00Z")
	s.Visits.SetNow(func() time.Time { return clockIn })

	msg := s.expectError(s.do(http.MethodPost, path+"/clock-out", token, nil), http.StatusConflict)
	if msg == "" {
		t.Error("clock-out before clock-in carried no message")
	}
	var got visitView
	s.decode(s.do(http.MethodPost, path+"/clock-in", token, nil), http.StatusOK, &got)
	if got.Status != models.VisitStatusInProgress || len(got.ClockEvents) != 1 {
		t.Fatalf("after clock-in: %+v", got)
	}
	event := got.ClockEvents[0]
	if event.Kind != models.ClockKindIn || event.OccurredAt != "2026-07-14T08:03:00Z" || event.RecordedByID != tom.ID || event.RecordedBy.FirstName != "Tom" {
		t.Errorf("clock-in event = %+v", event)
	}
	s.expectError(s.do(http.MethodPost, path+"/clock-in", token, nil), http.StatusConflict)

	clockOut := clockIn.Add(50 * time.Minute)
	s.Visits.SetNow(func() time.Time { return clockOut })
	s.decode(s.do(http.MethodPost, path+"/clock-out", token, nil), http.StatusOK, &got)
	if got.Status != models.VisitStatusCompleted || len(got.ClockEvents) != 2 || got.ClockEvents[1].Kind != models.ClockKindOut {
		t.Fatalf("after clock-out: %+v", got)
	}
	s.expectError(s.do(http.MethodPost, path+"/clock-out", token, nil), http.StatusConflict)
	s.expectError(s.do(http.MethodPost, path+"/clock-in", token, nil), http.StatusConflict)
	s.expectError(s.do(http.MethodPost, "/api/visits/999/clock-in", token, nil), http.StatusNotFound)
}

// rule: §4.1 — an admin may clock any visit and the event is recorded as theirs
func TestVisits_AdminClocksAnyVisit(t *testing.T) {
	s := newTestServer(t)
	admin := s.createUser("ada.admin@example.com", models.RoleAdmin, "")
	tom := s.createUser("tom.field@example.com", models.RoleTechnician, "")
	site := s.createSite("Depot")
	visit := s.createVisit(site.ID, &tom.ID, "2026-07-14T08:00:00Z", 60)
	var got visitView
	s.decode(s.do(http.MethodPost, "/api/visits/"+itoa(visit.ID)+"/clock-in", s.login(admin.Email), nil), http.StatusOK, &got)
	if got.ClockEvents[0].RecordedByID != admin.ID || got.TechnicianID == nil || *got.TechnicianID != tom.ID {
		t.Errorf("admin clock-in = %+v", got)
	}
}

// rule: §3.4 — cancelling keeps the row and records who and when
func TestVisits_CancelRecordsWhoAndWhen(t *testing.T) {
	s := newTestServer(t)
	dispatcher := s.createUser("dev.dispatcher@example.com", models.RoleDispatcher, "")
	tom := s.createUser("tom.field@example.com", models.RoleTechnician, "")
	site := s.createSite("Depot")
	visit := s.createVisit(site.ID, &tom.ID, "2026-07-14T08:00:00Z", 60)
	token := s.login(dispatcher.Email)
	at, _ := time.Parse(time.RFC3339, "2026-07-13T17:45:00Z")
	s.Visits.SetNow(func() time.Time { return at })

	var got visitView
	s.decode(s.do(http.MethodPost, "/api/visits/"+itoa(visit.ID)+"/cancel", token, nil), http.StatusOK, &got)
	if got.Status != models.VisitStatusCancelled || got.CancelledByID == nil || *got.CancelledByID != dispatcher.ID || got.CancelledAt == nil || *got.CancelledAt != "2026-07-13T17:45:00Z" {
		t.Errorf("cancelled visit = %+v", got)
	}
	s.expectError(s.do(http.MethodPost, "/api/visits/"+itoa(visit.ID)+"/cancel", token, nil), http.StatusConflict)
	s.expectError(s.do(http.MethodPost, "/api/visits/"+itoa(visit.ID)+"/clock-in", s.login(tom.Email), nil), http.StatusConflict)
	s.expectError(s.do(http.MethodPut, "/api/visits/"+itoa(visit.ID), token, map[string]any{
		"site_id": site.ID, "scheduled_start": "2026-07-14T09:00:00Z", "scheduled_end": "2026-07-14T10:00:00Z",
	}), http.StatusConflict)
	s.expectError(s.do(http.MethodPost, "/api/visits/999/cancel", token, nil), http.StatusNotFound)

	var list visitList
	s.decode(s.do(http.MethodGet, "/api/visits?from=2026-07-14&to=2026-07-14&status=cancelled", token, nil), http.StatusOK, &list)
	if list.Total != 1 {
		t.Errorf("cancelled filter total = %d, want 1", list.Total)
	}
}

// rule: §3.1 — a visit needs a live site, an optional active technician and a start before its end
func TestVisits_CreateAndUpdateValidation(t *testing.T) {
	s := newTestServer(t)
	dispatcher := s.createUser("dev.dispatcher@example.com", models.RoleDispatcher, "")
	tom := s.createUser("tom.field@example.com", models.RoleTechnician, "")
	site := s.createSite("Depot")
	token := s.login(dispatcher.Email)
	body := func(over map[string]any) map[string]any {
		b := map[string]any{"site_id": site.ID, "technician_id": tom.ID, "scheduled_start": "2026-07-14T08:00:00Z", "scheduled_end": "2026-07-14T09:00:00Z"}
		for k, v := range over {
			b[k] = v
		}
		return b
	}
	var created visitView
	s.decode(s.do(http.MethodPost, "/api/visits", token, body(nil)), http.StatusCreated, &created)
	if created.Status != models.VisitStatusScheduled || created.Technician == nil || created.Site.Name != "Depot" || len(created.ClockEvents) != 0 {
		t.Errorf("created = %+v", created)
	}
	for _, bad := range []map[string]any{
		body(map[string]any{"scheduled_end": "2026-07-14T08:00:00Z"}),
		body(map[string]any{"scheduled_start": "2026-07-14"}),
		body(map[string]any{"scheduled_start": "2026-07-14T08:00:00"}),
		body(map[string]any{"site_id": 999}),
		body(map[string]any{"technician_id": dispatcher.ID}),
		body(map[string]any{"technician_id": 999}),
		{"site_id": site.ID},
	} {
		s.expectError(s.do(http.MethodPost, "/api/visits", token, bad), http.StatusBadRequest)
	}

	var updated visitView
	s.decode(s.do(http.MethodPut, "/api/visits/"+itoa(created.ID), token, body(map[string]any{"technician_id": nil, "scheduled_start": "2026-07-14T10:00:00+01:00", "scheduled_end": "2026-07-14T11:00:00+01:00"})), http.StatusOK, &updated)
	if updated.TechnicianID != nil || updated.Technician != nil || updated.ScheduledStart != "2026-07-14T09:00:00Z" || updated.ScheduledEnd != "2026-07-14T10:00:00Z" {
		t.Errorf("updated = %+v", updated)
	}
	s.expectError(s.do(http.MethodPut, "/api/visits/999", token, body(nil)), http.StatusNotFound)
}

// rule: §3.7 — priority is low, normal or high; a create or edit that leaves it out gets normal, and any other value is rejected
func TestVisits_PriorityOnCreateAndEdit(t *testing.T) {
	s := newTestServer(t)
	dispatcher := s.createUser("dev.dispatcher@example.com", models.RoleDispatcher, "")
	site := s.createSite("Depot")
	token := s.login(dispatcher.Email)
	body := func(over map[string]any) map[string]any {
		b := map[string]any{"site_id": site.ID, "scheduled_start": "2026-07-14T08:00:00Z", "scheduled_end": "2026-07-14T09:00:00Z"}
		for k, v := range over {
			b[k] = v
		}
		return b
	}

	var plain, urgent visitView
	s.decode(s.do(http.MethodPost, "/api/visits", token, body(nil)), http.StatusCreated, &plain)
	if plain.Priority != models.VisitPriorityNormal {
		t.Errorf("created without priority: priority = %q, want %q", plain.Priority, models.VisitPriorityNormal)
	}
	s.decode(s.do(http.MethodPost, "/api/visits", token, body(map[string]any{"priority": models.VisitPriorityHigh})), http.StatusCreated, &urgent)
	if urgent.Priority != models.VisitPriorityHigh {
		t.Errorf("created with high: priority = %q, want %q", urgent.Priority, models.VisitPriorityHigh)
	}

	path := "/api/visits/" + itoa(urgent.ID)
	for _, bad := range []string{"urgent", "HIGH"} {
		if msg := s.expectError(s.do(http.MethodPost, "/api/visits", token, body(map[string]any{"priority": bad})), http.StatusBadRequest); !strings.Contains(msg, "unknown priority") {
			t.Errorf("create with priority %q: error = %q, want it to name the unknown priority", bad, msg)
		}
		s.expectError(s.do(http.MethodPut, path, token, body(map[string]any{"priority": bad})), http.StatusBadRequest)
	}
	var got visitView
	s.decode(s.do(http.MethodGet, path, token, nil), http.StatusOK, &got)
	if got.Priority != models.VisitPriorityHigh {
		t.Errorf("after rejected edits: priority = %q, want %q unchanged", got.Priority, models.VisitPriorityHigh)
	}

	s.decode(s.do(http.MethodPut, path, token, body(map[string]any{"priority": models.VisitPriorityLow})), http.StatusOK, &got)
	if got.Priority != models.VisitPriorityLow {
		t.Errorf("edited to low: priority = %q, want %q", got.Priority, models.VisitPriorityLow)
	}
	for _, edit := range []map[string]any{body(nil), body(map[string]any{"priority": nil})} {
		s.decode(s.do(http.MethodPut, path, token, body(map[string]any{"priority": models.VisitPriorityHigh})), http.StatusOK, nil)
		s.decode(s.do(http.MethodPut, path, token, edit), http.StatusOK, &got)
		if got.Priority != models.VisitPriorityNormal {
			t.Errorf("edit %v: priority = %q, want %q because an edit replaces every field, as it does for technician_id", edit, got.Priority, models.VisitPriorityNormal)
		}
	}
}

// rule: §3.7 — ?priority= narrows the list server-side; an unknown value is rejected, not answered with an empty list
func TestVisits_PriorityFilter(t *testing.T) {
	s := newTestServer(t)
	dispatcher := s.createUser("dev.dispatcher@example.com", models.RoleDispatcher, "")
	site := s.createSite("Depot")
	token := s.login(dispatcher.Email)
	book := func(start, priority string) {
		b := map[string]any{"site_id": site.ID, "scheduled_start": start, "scheduled_end": strings.Replace(start, ":00:00Z", ":30:00Z", 1)}
		if priority != "" {
			b["priority"] = priority
		}
		s.decode(s.do(http.MethodPost, "/api/visits", token, b), http.StatusCreated, nil)
	}
	book("2026-07-14T08:00:00Z", models.VisitPriorityHigh)
	book("2026-07-14T09:00:00Z", models.VisitPriorityLow)
	book("2026-07-14T10:00:00Z", "")
	book("2026-07-15T08:00:00Z", models.VisitPriorityHigh)

	for _, priority := range models.AllVisitPriorities {
		var list visitList
		s.decode(s.do(http.MethodGet, "/api/visits?from=2026-07-14&to=2026-07-14&priority="+priority, token, nil), http.StatusOK, &list)
		if list.Total != 1 || len(list.Visits) != 1 || list.Visits[0].Priority != priority {
			t.Errorf("?priority=%s on 2026-07-14: total %d, visits %+v, want the one %s visit on that day", priority, list.Total, list.Visits, priority)
		}
	}
	var all visitList
	s.decode(s.do(http.MethodGet, "/api/visits?from=2026-07-14&to=2026-07-14&priority=", token, nil), http.StatusOK, &all)
	if all.Total != 3 {
		t.Errorf("empty ?priority=: total %d, want 3 (no filter)", all.Total)
	}
	for _, bad := range []string{"urgent", "high,low"} {
		if msg := s.expectError(s.do(http.MethodGet, "/api/visits?from=2026-07-14&to=2026-07-14&priority="+bad, token, nil), http.StatusBadRequest); !strings.Contains(msg, "unknown priority") {
			t.Errorf("?priority=%s: error = %q, want it to name the unknown priority", bad, msg)
		}
	}
}

package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nwasiq/fieldops/backend/internal/models"
)

// rule: §6.1 — a PUT writes an audit row whose changes carry the real before and after of each changed field
func TestAudit_UpdateDiff(t *testing.T) {
	s := newTestServer(t)
	admin := s.createUser("ada.admin@example.com", models.RoleAdmin, "")
	token := s.login(admin.Email)
	site := s.createSite("Depot")

	s.decode(s.do(http.MethodPut, "/api/sites/"+itoa(site.ID), token, map[string]any{
		"name": "Depot North", "address": site.Address, "contact_phone": "0200",
	}), http.StatusOK, nil)

	rows := s.auditRows(models.ResourceSite, site.ID)
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d, want 1 (the service-created site is not a request)", len(rows))
	}
	row := rows[0]
	if row.ActorID != admin.ID || row.Action != models.AuditActionUpdate || row.ResourceType != models.ResourceSite {
		t.Errorf("row = %+v", row)
	}
	changes := changesOf(t, row)
	if len(changes) != 2 {
		t.Fatalf("changes = %v, want name and contact_phone only", changes)
	}
	if c := changes["name"]; c.Before != "Depot" || c.After != "Depot North" {
		t.Errorf("name change = %+v", c)
	}
	if c := changes["contact_phone"]; c.Before != "0100" || c.After != "0200" {
		t.Errorf("contact_phone change = %+v", c)
	}

	s.expectError(s.do(http.MethodPut, "/api/sites/"+itoa(site.ID), token, map[string]any{"name": ""}), http.StatusBadRequest)
	if got := len(s.auditRows(models.ResourceSite, site.ID)); got != 1 {
		t.Errorf("a rejected PUT wrote an audit row (%d rows)", got)
	}
}

// rule: §6.1 — a clock-in writes an audit row recording the status move and the clock instant
func TestAudit_ClockIn(t *testing.T) {
	s := newTestServer(t)
	tom := s.createUser("tom.field@example.com", models.RoleTechnician, "")
	site := s.createSite("Depot")
	visit := s.createVisit(site.ID, &tom.ID, "2026-07-14T08:00:00Z", 60)
	at, _ := time.Parse(time.RFC3339, "2026-07-14T08:02:00Z")
	s.Visits.SetNow(func() time.Time { return at })

	s.decode(s.do(http.MethodPost, "/api/visits/"+itoa(visit.ID)+"/clock-in", s.login(tom.Email), nil), http.StatusOK, nil)
	rows := s.auditRows(models.ResourceVisit, visit.ID)
	if len(rows) != 1 || rows[0].Action != models.AuditActionClockIn || rows[0].ActorID != tom.ID {
		t.Fatalf("rows = %+v", rows)
	}
	changes := changesOf(t, rows[0])
	if c := changes["status"]; c.Before != models.VisitStatusScheduled || c.After != models.VisitStatusInProgress {
		t.Errorf("status change = %+v", c)
	}
	if c := changes["clock_in_at"]; c.Before != nil || c.After != "2026-07-14T08:02:00Z" {
		t.Errorf("clock_in_at change = %+v", c)
	}
	if _, present := changes["site_id"]; present {
		t.Error("an unchanged field was reported")
	}
}

// rule: §6.1 — create, delete, cancel and login are audited; the licence number is fingerprinted, never stored in the clear
func TestAudit_CreateDeleteAndLogin(t *testing.T) {
	s := newTestServer(t)
	admin := s.createUser("ada.admin@example.com", models.RoleAdmin, "")
	token := s.login(admin.Email)

	var created map[string]any
	s.decode(s.do(http.MethodPost, "/api/users", token, map[string]any{
		"email": "tom.field@example.com", "first_name": "Tom", "last_name": "Field",
		"role": models.RoleTechnician, "password": "longenough", "licence_number": "GS-284611",
	}), http.StatusCreated, &created)
	id := uint(created["id"].(float64))
	rows := s.auditRows(models.ResourceUser, id)
	if len(rows) != 1 || rows[0].Action != models.AuditActionCreate {
		t.Fatalf("create rows = %+v", rows)
	}
	changes := changesOf(t, rows[0])
	if c := changes["email"]; c.Before != nil || c.After != "tom.field@example.com" {
		t.Errorf("create diff email = %+v", c)
	}
	if strings.Contains(rows[0].Changes, "GS-284611") {
		t.Error("audit row holds the licence number in the clear")
	}
	if c, ok := changes["licence_number"]; !ok || c.After == "" {
		t.Errorf("licence_number change missing or empty: %+v", c)
	}

	s.decode(s.do(http.MethodDelete, "/api/users/"+itoa(id), token, nil), http.StatusOK, nil)
	rows = s.auditRows(models.ResourceUser, id)
	if len(rows) != 2 || rows[1].Action != models.AuditActionDelete {
		t.Fatalf("delete rows = %+v", rows)
	}
	if c := changesOf(t, rows[1])["deleted_at"]; c.Before != nil || c.After == nil {
		t.Errorf("delete diff = %+v, want deleted_at set", c)
	}

	s.decode(s.do(http.MethodPost, "/api/auth/login", "", map[string]string{"email": admin.Email, "password": testPassword}), http.StatusOK, nil)
	loginRows := s.auditRows(models.ResourceUser, admin.ID)
	if len(loginRows) != 1 || loginRows[0].Action != models.AuditActionLogin || loginRows[0].ActorID != admin.ID || loginRows[0].Changes != "{}" {
		t.Errorf("login rows = %+v", loginRows)
	}
	s.expectError(s.do(http.MethodPost, "/api/auth/login", "", map[string]string{"email": admin.Email, "password": "wrong"}), http.StatusUnauthorized)
	if got := len(s.auditRows(models.ResourceUser, admin.ID)); got != 1 {
		t.Errorf("a failed login wrote an audit row (%d rows)", got)
	}
}

// rule: §6.1 — every mutating route in the router has an auditor registered under its exact pattern
func TestAudit_RegistryCoversEveryMutatingRoute(t *testing.T) {
	s := newTestServer(t)
	registered := map[string]bool{}
	for _, key := range s.Registry.Registered() {
		registered[key] = true
	}
	seen := map[string]bool{}
	for _, route := range s.Engine.Routes() {
		switch route.Method {
		case http.MethodPost, http.MethodPut, http.MethodDelete:
			key := route.Method + " " + route.Path
			seen[key] = true
			if !registered[key] {
				t.Errorf("mutating route %s has no auditor", key)
			}
		}
	}
	for key := range registered {
		if !seen[key] {
			t.Errorf("auditor %s has no matching route", key)
		}
	}
}

// rule: §6.1 — a priority change is audited with its before and after values
func TestAudit_VisitPriorityChange(t *testing.T) {
	s := newTestServer(t)
	dispatcher := s.createUser("dev.dispatcher@example.com", models.RoleDispatcher, "")
	site := s.createSite("Depot")
	visit := s.createVisit(site.ID, nil, "2026-07-14T08:00:00Z", 60)

	s.decode(s.do(http.MethodPut, "/api/visits/"+itoa(visit.ID), s.login(dispatcher.Email), map[string]any{
		"site_id": site.ID, "scheduled_start": "2026-07-14T08:00:00Z", "scheduled_end": "2026-07-14T09:00:00Z", "priority": models.VisitPriorityHigh,
	}), http.StatusOK, nil)
	rows := s.auditRows(models.ResourceVisit, visit.ID)
	if len(rows) != 1 || rows[0].Action != models.AuditActionUpdate {
		t.Fatalf("rows = %+v", rows)
	}
	changes := changesOf(t, rows[0])
	if c := changes["priority"]; c.Before != models.VisitPriorityNormal || c.After != models.VisitPriorityHigh {
		t.Errorf("priority change = %+v, want normal to high", c)
	}
	if _, present := changes["scheduled_start"]; present {
		t.Error("an unchanged field was reported")
	}
}

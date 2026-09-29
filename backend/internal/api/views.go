package api

import (
	"time"

	"github.com/nwasiq/fieldops/backend/internal/models"
	"github.com/nwasiq/fieldops/backend/internal/services"
)

// The views are the API's wire shapes. Models never serialise directly, so a
// column added to a struct cannot leak into a response by accident.

type userView struct {
	ID            uint    `json:"id"`
	Email         string  `json:"email"`
	FirstName     string  `json:"first_name"`
	LastName      string  `json:"last_name"`
	Role          string  `json:"role"`
	IsActive      bool    `json:"is_active"`
	LicenceNumber *string `json:"licence_number,omitempty"`
	CreatedAt     string  `json:"created_at"`
}

type userRef struct {
	ID        uint   `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type siteView struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Address      string `json:"address"`
	ContactPhone string `json:"contact_phone"`
}

type siteRef struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type clockEventView struct {
	ID           uint    `json:"id"`
	Kind         string  `json:"kind"`
	OccurredAt   string  `json:"occurred_at"`
	RecordedByID uint    `json:"recorded_by_id"`
	RecordedBy   userRef `json:"recorded_by"`
}

type visitView struct {
	ID             uint             `json:"id"`
	SiteID         uint             `json:"site_id"`
	Site           siteRef          `json:"site"`
	TechnicianID   *uint            `json:"technician_id"`
	Technician     *userRef         `json:"technician"`
	ScheduledStart string           `json:"scheduled_start"`
	ScheduledEnd   string           `json:"scheduled_end"`
	Status         string           `json:"status"`
	Priority       string           `json:"priority"`
	CancelledByID  *uint            `json:"cancelled_by_id"`
	CancelledAt    *string          `json:"cancelled_at"`
	ClockEvents    []clockEventView `json:"clock_events"`
}

type pageView struct {
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
}

type userListView struct {
	Users []userView `json:"users"`
	pageView
}

type siteListView struct {
	Sites []siteView `json:"sites"`
	pageView
}

type visitListView struct {
	Visits []visitView `json:"visits"`
	pageView
}

type technicianDayView struct {
	TechnicianID uint    `json:"technician_id"`
	Name         string  `json:"name"`
	Visits       int     `json:"visits"`
	Completed    int     `json:"completed"`
	HoursClocked float64 `json:"hours_clocked"`
}

type dailyReportView struct {
	Date        string              `json:"date"`
	Scheduled   int                 `json:"scheduled"`
	Completed   int                 `json:"completed"`
	Cancelled   int                 `json:"cancelled"`
	Technicians []technicianDayView `json:"technicians"`
}

func instant(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func optionalInstant(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := instant(*t)
	return &s
}

// newUserView renders a user. The licence number is present only when the
// service supplied it, which it does for admin detail reads alone (§1.3).
func newUserView(u *models.User, withLicence bool) userView {
	view := userView{
		ID:        u.ID,
		Email:     u.Email,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Role:      u.Role,
		IsActive:  u.IsActive,
		CreatedAt: instant(u.CreatedAt),
	}
	if withLicence {
		licence := u.LicenceNumber
		view.LicenceNumber = &licence
	}
	return view
}

func newUserRef(u *models.User) *userRef {
	if u == nil {
		return nil
	}
	return &userRef{ID: u.ID, FirstName: u.FirstName, LastName: u.LastName}
}

func newSiteView(s *models.Site) siteView {
	return siteView{ID: s.ID, Name: s.Name, Address: s.Address, ContactPhone: s.ContactPhone}
}

func newVisitView(v *models.Visit) visitView {
	events := make([]clockEventView, 0, len(v.ClockEvents))
	for i := range v.ClockEvents {
		e := &v.ClockEvents[i]
		events = append(events, clockEventView{
			ID:           e.ID,
			Kind:         e.Kind,
			OccurredAt:   instant(e.OccurredAt),
			RecordedByID: e.RecordedByID,
			RecordedBy:   *newUserRef(&e.RecordedBy),
		})
	}
	return visitView{
		ID:             v.ID,
		SiteID:         v.SiteID,
		Site:           siteRef{ID: v.Site.ID, Name: v.Site.Name},
		TechnicianID:   v.TechnicianID,
		Technician:     newUserRef(v.Technician),
		ScheduledStart: instant(v.ScheduledStart),
		ScheduledEnd:   instant(v.ScheduledEnd),
		Status:         v.Status,
		Priority:       v.Priority,
		CancelledByID:  v.CancelledByID,
		CancelledAt:    optionalInstant(v.CancelledAt),
		ClockEvents:    events,
	}
}

func newPageView[T any](result services.PageResult[T]) pageView {
	return pageView{Total: result.Total, Page: result.Page, PageSize: result.PageSize, TotalPages: result.TotalPages}
}

func newDailyReportView(r *services.DailyReport) dailyReportView {
	technicians := make([]technicianDayView, 0, len(r.Technicians))
	for _, t := range r.Technicians {
		technicians = append(technicians, technicianDayView(t))
	}
	return dailyReportView{
		Date:        r.Date,
		Scheduled:   r.Scheduled,
		Completed:   r.Completed,
		Cancelled:   r.Cancelled,
		Technicians: technicians,
	}
}

package services

import (
	"context"
	"fmt"
	"math"
	"sort"

	"gorm.io/gorm"

	"github.com/nwasiq/fieldops/backend/internal/models"
)

// ReportService builds the daily report (§5).
type ReportService struct {
	db *gorm.DB
}

// NewReportService builds a ReportService.
func NewReportService(db *gorm.DB) *ReportService {
	return &ReportService{db: db}
}

// DailyReport summarises one Europe/London day.
type DailyReport struct {
	Date        string
	Scheduled   int
	Completed   int
	Cancelled   int
	Technicians []TechnicianDay
}

// TechnicianDay is one technician's line in the daily report.
type TechnicianDay struct {
	TechnicianID uint
	Name         string
	Visits       int
	Completed    int
	HoursClocked float64
}

// Daily reports the visits whose scheduled start falls on the given
// Europe/London day (§5.1). Scheduled is the day's whole book, every status;
// Completed and Cancelled are the subsets in those states. A technician with
// visits that day is listed even if since deactivated or deleted (§5.2).
func (s *ReportService) Daily(ctx context.Context, day string) (*DailyReport, error) {
	start, end, err := UKDayRange(day)
	if err != nil {
		return nil, invalidf("date must be YYYY-MM-DD")
	}
	var visits []models.Visit
	err = s.db.WithContext(ctx).
		Where("scheduled_start >= ? AND scheduled_start < ?", start, end).
		Preload("Technician", UnscopedUserAssoc).
		Preload("ClockEvents", func(db *gorm.DB) *gorm.DB { return db.Order("occurred_at, id") }).
		Order("scheduled_start, id").
		Find(&visits).Error
	if err != nil {
		return nil, fmt.Errorf("load visits for report: %w", err)
	}

	report := &DailyReport{Date: day, Technicians: []TechnicianDay{}}
	lines := map[uint]*TechnicianDay{}
	for _, visit := range visits {
		report.Scheduled++
		switch visit.Status {
		case models.VisitStatusCompleted:
			report.Completed++
		case models.VisitStatusCancelled:
			report.Cancelled++
		}
		if visit.TechnicianID == nil || visit.Technician == nil {
			continue
		}
		line, ok := lines[*visit.TechnicianID]
		if !ok {
			line = &TechnicianDay{TechnicianID: *visit.TechnicianID, Name: visit.Technician.FullName()}
			lines[*visit.TechnicianID] = line
		}
		line.Visits++
		if visit.Status == models.VisitStatusCompleted {
			line.Completed++
		}
		line.HoursClocked += hoursClocked(visit.ClockEvents)
	}
	for _, line := range lines {
		line.HoursClocked = math.Round(line.HoursClocked*100) / 100
		report.Technicians = append(report.Technicians, *line)
	}
	sort.Slice(report.Technicians, func(i, j int) bool {
		a, b := report.Technicians[i], report.Technicians[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.TechnicianID < b.TechnicianID
	})
	return report, nil
}

// hoursClocked sums the time between each clock-in and the clock-out that
// follows it. An open clock-in contributes nothing until it is closed.
func hoursClocked(events []models.ClockEvent) float64 {
	var hours float64
	var open *models.ClockEvent
	for i := range events {
		event := &events[i]
		switch event.Kind {
		case models.ClockKindIn:
			open = event
		case models.ClockKindOut:
			if open != nil {
				hours += event.OccurredAt.Sub(open.OccurredAt).Hours()
				open = nil
			}
		}
	}
	return hours
}

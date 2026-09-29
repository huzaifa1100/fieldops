package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/nwasiq/fieldops/backend/internal/models"
)

// VisitService schedules visits and records clock events (§3, §4).
type VisitService struct {
	db  *gorm.DB
	now func() time.Time
}

// NewVisitService builds a VisitService.
func NewVisitService(db *gorm.DB) *VisitService {
	return &VisitService{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// SetNow overrides the clock; tests use it to pin clock event instants.
func (s *VisitService) SetNow(now func() time.Time) {
	s.now = now
}

// VisitFilter narrows a visit list. From and To are Europe/London days and
// are required (§3.2).
type VisitFilter struct {
	From         string
	To           string
	TechnicianID *uint
	Status       string
	Priority     string
}

// VisitInput is the writable shape of a visit. Status is never set directly:
// it moves through cancel and clock actions only. An empty Priority means
// normal, on edits too, because an edit replaces every field (§3.7).
type VisitInput struct {
	SiteID         uint
	TechnicianID   *uint
	ScheduledStart time.Time
	ScheduledEnd   time.Time
	Priority       string
}

func (in *VisitInput) validate() error {
	switch {
	case in.SiteID == 0:
		return invalidf("site_id is required")
	case in.ScheduledStart.IsZero() || in.ScheduledEnd.IsZero():
		return invalidf("scheduled_start and scheduled_end are required")
	case !in.ScheduledEnd.After(in.ScheduledStart):
		return invalidf("scheduled_end must be after scheduled_start")
	case in.Priority != "" && !models.IsValidVisitPriority(in.Priority):
		return invalidf("unknown priority %q", in.Priority)
	}
	if in.Priority == "" {
		in.Priority = models.VisitPriorityNormal
	}
	in.ScheduledStart = in.ScheduledStart.UTC()
	in.ScheduledEnd = in.ScheduledEnd.UTC()
	return nil
}

// withAssociations preloads everything the visit JSON shows. Attribution
// associations are unscoped so departed users and deleted sites still resolve.
func withAssociations(db *gorm.DB) *gorm.DB {
	return db.
		Preload("Site", UnscopedSiteAssoc).
		Preload("Technician", UnscopedUserAssoc).
		Preload("CancelledBy", UnscopedUserAssoc).
		Preload("ClockEvents", func(db *gorm.DB) *gorm.DB { return db.Order("occurred_at, id") }).
		Preload("ClockEvents.RecordedBy", UnscopedUserAssoc)
}

// List returns visits whose scheduled start falls within the inclusive run of
// Europe/London days from..to (§3.2, §3.3). A technician only ever sees their
// own visits; the filter is forced here, not trusted from the request.
func (s *VisitService) List(ctx context.Context, actor Actor, filter VisitFilter, page Page) (PageResult[models.Visit], error) {
	if filter.From == "" || filter.To == "" {
		return PageResult[models.Visit]{}, invalidf("from and to are required (YYYY-MM-DD)")
	}
	start, end, err := UKDaySpan(filter.From, filter.To)
	if err != nil {
		return PageResult[models.Visit]{}, invalidf("%s", err.Error())
	}
	if filter.Status != "" && !models.IsValidVisitStatus(filter.Status) {
		return PageResult[models.Visit]{}, invalidf("unknown status %q", filter.Status)
	}
	if filter.Priority != "" && !models.IsValidVisitPriority(filter.Priority) {
		return PageResult[models.Visit]{}, invalidf("unknown priority %q", filter.Priority)
	}
	if actor.IsTechnician() {
		id := actor.ID
		filter.TechnicianID = &id
	}

	query := s.db.WithContext(ctx).Model(&models.Visit{}).
		Where("scheduled_start >= ? AND scheduled_start < ?", start, end)
	if filter.TechnicianID != nil {
		query = query.Where("technician_id = ?", *filter.TechnicianID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Priority != "" {
		query = query.Where("priority = ?", filter.Priority)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return PageResult[models.Visit]{}, fmt.Errorf("count visits: %w", err)
	}
	var visits []models.Visit
	err = withAssociations(query).Order("scheduled_start, id").Offset(page.Offset()).Limit(page.Size).Find(&visits).Error
	if err != nil {
		return PageResult[models.Visit]{}, fmt.Errorf("list visits: %w", err)
	}
	return newPageResult(visits, total, page), nil
}

// Get returns one visit with its associations. A technician may only read a
// visit assigned to them (§1.1).
func (s *VisitService) Get(ctx context.Context, actor Actor, id uint) (*models.Visit, error) {
	visit, err := s.load(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	if err := assertOwnVisit(actor, visit); err != nil {
		return nil, err
	}
	return visit, nil
}

// Create schedules a visit in status scheduled.
func (s *VisitService) Create(ctx context.Context, in VisitInput) (*models.Visit, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	if err := s.checkReferences(ctx, in); err != nil {
		return nil, err
	}
	visit := models.Visit{
		SiteID:         in.SiteID,
		TechnicianID:   in.TechnicianID,
		ScheduledStart: in.ScheduledStart,
		ScheduledEnd:   in.ScheduledEnd,
		Status:         models.VisitStatusScheduled,
		Priority:       in.Priority,
	}
	if err := s.db.WithContext(ctx).Create(&visit).Error; err != nil {
		return nil, fmt.Errorf("create visit: %w", err)
	}
	return s.load(ctx, s.db, visit.ID)
}

// Update replaces the schedulable fields of a visit that is still open. A
// completed or cancelled visit is closed to edits (§3.5).
func (s *VisitService) Update(ctx context.Context, id uint, in VisitInput) (*models.Visit, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	var visit models.Visit
	err := s.db.WithContext(ctx).First(&visit, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFound("visit")
	}
	if err != nil {
		return nil, fmt.Errorf("get visit: %w", err)
	}
	if isClosed(visit.Status) {
		return nil, conflict(fmt.Sprintf("a %s visit cannot be edited", visit.Status))
	}
	if err := s.checkReferences(ctx, in); err != nil {
		return nil, err
	}
	visit.SiteID = in.SiteID
	visit.TechnicianID = in.TechnicianID
	visit.ScheduledStart = in.ScheduledStart
	visit.ScheduledEnd = in.ScheduledEnd
	visit.Priority = in.Priority
	if err := s.db.WithContext(ctx).Save(&visit).Error; err != nil {
		return nil, fmt.Errorf("update visit: %w", err)
	}
	return s.load(ctx, s.db, visit.ID)
}

// Cancel marks an open visit cancelled and records who did it and when (§3.4).
func (s *VisitService) Cancel(ctx context.Context, id uint, actor Actor) (*models.Visit, error) {
	now := s.now()
	result := s.db.WithContext(ctx).Model(&models.Visit{}).
		Where("id = ? AND status IN ?", id, []string{models.VisitStatusScheduled, models.VisitStatusInProgress}).
		Updates(map[string]any{
			"status":          models.VisitStatusCancelled,
			"cancelled_by_id": actor.ID,
			"cancelled_at":    now,
			"updated_at":      now,
		})
	if result.Error != nil {
		return nil, fmt.Errorf("cancel visit: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, s.explainNoTransition(ctx, id, "cancelled")
	}
	return s.load(ctx, s.db, id)
}

// ClockIn moves a scheduled visit to in_progress and records the event (§4.2).
func (s *VisitService) ClockIn(ctx context.Context, id uint, actor Actor) (*models.Visit, error) {
	return s.clock(ctx, id, actor, models.ClockKindIn, models.VisitStatusScheduled, models.VisitStatusInProgress)
}

// ClockOut moves an in_progress visit to completed and records the event (§4.2).
func (s *VisitService) ClockOut(ctx context.Context, id uint, actor Actor) (*models.Visit, error) {
	return s.clock(ctx, id, actor, models.ClockKindOut, models.VisitStatusInProgress, models.VisitStatusCompleted)
}

// clock is the single path for both clock directions. The status move is a
// conditional update so two concurrent requests cannot both succeed: the
// second finds no row in the expected state and is refused with 409.
func (s *VisitService) clock(ctx context.Context, id uint, actor Actor, kind, fromStatus, toStatus string) (*models.Visit, error) {
	var visit models.Visit
	err := s.db.WithContext(ctx).First(&visit, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFound("visit")
	}
	if err != nil {
		return nil, fmt.Errorf("get visit: %w", err)
	}
	if err := assertOwnVisit(actor, &visit); err != nil {
		return nil, err
	}
	if visit.Status != fromStatus {
		return nil, clockConflict(kind, visit.Status)
	}
	now := s.now()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.Visit{}).
			Where("id = ? AND status = ?", id, fromStatus).
			Updates(map[string]any{"status": toStatus, "updated_at": now})
		if result.Error != nil {
			return fmt.Errorf("move visit to %s: %w", toStatus, result.Error)
		}
		if result.RowsAffected == 0 {
			return clockConflict(kind, visit.Status)
		}
		event := models.ClockEvent{VisitID: id, Kind: kind, OccurredAt: now, RecordedByID: actor.ID}
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("record clock %s: %w", kind, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.load(ctx, s.db, id)
}

func clockConflict(kind, status string) error {
	switch {
	case kind == models.ClockKindIn && status == models.VisitStatusInProgress:
		return conflict("visit is already clocked in")
	case kind == models.ClockKindOut && status == models.VisitStatusScheduled:
		return conflict("visit has not been clocked in")
	default:
		return conflict(fmt.Sprintf("cannot clock %s of a %s visit", kind, status))
	}
}

func (s *VisitService) explainNoTransition(ctx context.Context, id uint, target string) error {
	var visit models.Visit
	err := s.db.WithContext(ctx).First(&visit, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound("visit")
	}
	if err != nil {
		return fmt.Errorf("get visit: %w", err)
	}
	return conflict(fmt.Sprintf("a %s visit cannot be %s", visit.Status, target))
}

// load reads a visit with every association the API shows.
func (s *VisitService) load(ctx context.Context, db *gorm.DB, id uint) (*models.Visit, error) {
	var visit models.Visit
	err := withAssociations(db.WithContext(ctx)).First(&visit, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFound("visit")
	}
	if err != nil {
		return nil, fmt.Errorf("get visit: %w", err)
	}
	return &visit, nil
}

// checkReferences confirms the site is live and the technician, when set, is
// an active user holding the technician role (§3.6).
func (s *VisitService) checkReferences(ctx context.Context, in VisitInput) error {
	var siteCount int64
	if err := s.db.WithContext(ctx).Model(&models.Site{}).Where("id = ?", in.SiteID).Count(&siteCount).Error; err != nil {
		return fmt.Errorf("check site: %w", err)
	}
	if siteCount == 0 {
		return invalidf("site %d does not exist", in.SiteID)
	}
	if in.TechnicianID == nil {
		return nil
	}
	var tech models.User
	err := s.db.WithContext(ctx).First(&tech, *in.TechnicianID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return invalidf("technician %d does not exist", *in.TechnicianID)
	}
	if err != nil {
		return fmt.Errorf("check technician: %w", err)
	}
	if tech.Role != models.RoleTechnician || !tech.IsActive {
		return invalidf("user %d is not an active technician", *in.TechnicianID)
	}
	return nil
}

// assertOwnVisit enforces that a technician only touches their own visits (§4.1).
func assertOwnVisit(actor Actor, visit *models.Visit) error {
	if !actor.IsTechnician() {
		return nil
	}
	if visit.TechnicianID == nil || *visit.TechnicianID != actor.ID {
		return forbidden("visit is not assigned to you")
	}
	return nil
}

func isClosed(status string) bool {
	return status == models.VisitStatusCompleted || status == models.VisitStatusCancelled
}

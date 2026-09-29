package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"gorm.io/gorm"

	"github.com/nwasiq/fieldops/backend/internal/models"
)

// AuditService writes the append-only audit trail and takes the resource
// snapshots it is diffed from (§6).
type AuditService struct {
	db     *gorm.DB
	cipher *Cipher
}

// NewAuditService builds an AuditService.
func NewAuditService(db *gorm.DB, cipher *Cipher) *AuditService {
	return &AuditService{db: db, cipher: cipher}
}

// Snapshot is the audited view of one resource: JSON-shaped field values.
type Snapshot map[string]any

// Change is the before/after pair for one changed field.
type Change struct {
	Before any `json:"before"`
	After  any `json:"after"`
}

// Entry is one audit row to write.
type Entry struct {
	ActorID      uint
	Action       string
	ResourceType string
	ResourceID   uint
	Changes      map[string]Change
}

// Record appends an audit row. Rows are never updated or deleted (§6.2).
func (s *AuditService) Record(ctx context.Context, entry Entry) error {
	if entry.Changes == nil {
		entry.Changes = map[string]Change{}
	}
	changes, err := json.Marshal(entry.Changes)
	if err != nil {
		return fmt.Errorf("encode audit changes: %w", err)
	}
	row := models.AuditLog{
		ActorID:      entry.ActorID,
		Action:       entry.Action,
		ResourceType: entry.ResourceType,
		ResourceID:   entry.ResourceID,
		Changes:      string(changes),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}

// Diff lists the fields whose value differs between two snapshots. A nil
// before (the resource did not exist) diffs every populated field against
// null.
func Diff(before, after Snapshot) map[string]Change {
	changes := map[string]Change{}
	keys := map[string]struct{}{}
	for k := range before {
		keys[k] = struct{}{}
	}
	for k := range after {
		keys[k] = struct{}{}
	}
	for k := range keys {
		b, a := before[k], after[k]
		if !reflect.DeepEqual(b, a) {
			changes[k] = Change{Before: b, After: a}
		}
	}
	return changes
}

// SnapshotUser reads a user for the audit trail, soft-deleted or not. The
// licence number is reduced to a fingerprint so a change is visible without
// the value leaving its encrypted column. A missing user yields nil.
func (s *AuditService) SnapshotUser(ctx context.Context, id uint) (Snapshot, error) {
	var user models.User
	err := s.db.WithContext(ctx).Unscoped().First(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("snapshot user: %w", err)
	}
	if err := decryptUserSensitive(s.cipher, &user); err != nil {
		return nil, err
	}
	return toSnapshot(struct {
		ID            uint    `json:"id"`
		Email         string  `json:"email"`
		FirstName     string  `json:"first_name"`
		LastName      string  `json:"last_name"`
		Role          string  `json:"role"`
		IsActive      bool    `json:"is_active"`
		LicenceNumber string  `json:"licence_number"`
		DeletedAt     *string `json:"deleted_at"`
	}{
		ID:            user.ID,
		Email:         user.Email,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
		Role:          user.Role,
		IsActive:      user.IsActive,
		LicenceNumber: fingerprint(user.LicenceNumber),
		DeletedAt:     deletedAt(user.DeletedAt),
	})
}

// SnapshotSite reads a site for the audit trail, soft-deleted or not.
func (s *AuditService) SnapshotSite(ctx context.Context, id uint) (Snapshot, error) {
	var site models.Site
	err := s.db.WithContext(ctx).Unscoped().First(&site, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("snapshot site: %w", err)
	}
	return toSnapshot(struct {
		ID           uint    `json:"id"`
		Name         string  `json:"name"`
		Address      string  `json:"address"`
		ContactPhone string  `json:"contact_phone"`
		DeletedAt    *string `json:"deleted_at"`
	}{
		ID:           site.ID,
		Name:         site.Name,
		Address:      site.Address,
		ContactPhone: site.ContactPhone,
		DeletedAt:    deletedAt(site.DeletedAt),
	})
}

// SnapshotVisit reads a visit for the audit trail, including the instants of
// its clock events so a clock action shows as more than a status change.
func (s *AuditService) SnapshotVisit(ctx context.Context, id uint) (Snapshot, error) {
	var visit models.Visit
	err := s.db.WithContext(ctx).
		Preload("ClockEvents", func(db *gorm.DB) *gorm.DB { return db.Order("occurred_at, id") }).
		First(&visit, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("snapshot visit: %w", err)
	}
	var clockIn, clockOut *string
	for _, event := range visit.ClockEvents {
		stamp := formatInstant(event.OccurredAt)
		switch event.Kind {
		case models.ClockKindIn:
			clockIn = &stamp
		case models.ClockKindOut:
			clockOut = &stamp
		}
	}
	return toSnapshot(struct {
		ID             uint    `json:"id"`
		SiteID         uint    `json:"site_id"`
		TechnicianID   *uint   `json:"technician_id"`
		ScheduledStart string  `json:"scheduled_start"`
		ScheduledEnd   string  `json:"scheduled_end"`
		Status         string  `json:"status"`
		Priority       string  `json:"priority"`
		CancelledByID  *uint   `json:"cancelled_by_id"`
		CancelledAt    *string `json:"cancelled_at"`
		ClockInAt      *string `json:"clock_in_at"`
		ClockOutAt     *string `json:"clock_out_at"`
	}{
		ID:             visit.ID,
		SiteID:         visit.SiteID,
		TechnicianID:   visit.TechnicianID,
		ScheduledStart: formatInstant(visit.ScheduledStart),
		ScheduledEnd:   formatInstant(visit.ScheduledEnd),
		Status:         visit.Status,
		Priority:       visit.Priority,
		CancelledByID:  visit.CancelledByID,
		CancelledAt:    formatOptionalInstant(visit.CancelledAt),
		ClockInAt:      clockIn,
		ClockOutAt:     clockOut,
	})
}

// toSnapshot normalises a tagged struct through JSON so before and after
// compare as the same shapes (numbers as float64, times as strings).
func toSnapshot(v any) (Snapshot, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode snapshot: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	return snapshot, nil
}

func deletedAt(d gorm.DeletedAt) *string {
	if !d.Valid {
		return nil
	}
	stamp := formatInstant(d.Time)
	return &stamp
}

// formatInstant renders an instant as RFC3339 in UTC, the wire format for
// every timestamp the API emits.
func formatInstant(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func formatOptionalInstant(t *time.Time) *string {
	if t == nil {
		return nil
	}
	stamp := formatInstant(*t)
	return &stamp
}

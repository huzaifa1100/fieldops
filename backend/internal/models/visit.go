package models

import "time"

// Visit is a scheduled attendance at a site (§3.1). Visits are never deleted:
// a visit that will not happen is cancelled and keeps its row (§3.4).
type Visit struct {
	ID             uint      `gorm:"primaryKey"`
	SiteID         uint      `gorm:"not null;index"`
	Site           Site      `gorm:"foreignKey:SiteID"`
	TechnicianID   *uint     `gorm:"index"`
	Technician     *User     `gorm:"foreignKey:TechnicianID"`
	ScheduledStart time.Time `gorm:"not null;index"`
	ScheduledEnd   time.Time `gorm:"not null"`
	Status         string    `gorm:"size:32;not null;index"`
	Priority       string    `gorm:"size:16;not null;default:normal"`
	CancelledByID  *uint
	CancelledBy    *User `gorm:"foreignKey:CancelledByID"`
	CancelledAt    *time.Time
	ClockEvents    []ClockEvent `gorm:"foreignKey:VisitID"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ClockEvent records a technician clocking in or out of a visit (§4.3).
type ClockEvent struct {
	ID           uint      `gorm:"primaryKey"`
	VisitID      uint      `gorm:"not null;index"`
	Kind         string    `gorm:"size:8;not null"`
	OccurredAt   time.Time `gorm:"not null"`
	RecordedByID uint      `gorm:"not null"`
	RecordedBy   User      `gorm:"foreignKey:RecordedByID"`
	CreatedAt    time.Time
}

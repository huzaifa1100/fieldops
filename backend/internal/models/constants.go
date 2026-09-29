package models

// User roles (§1.1).
const (
	RoleAdmin      = "admin"
	RoleDispatcher = "dispatcher"
	RoleTechnician = "technician"
)

// AllRoles lists every role a user may hold, in the order the API documents them.
var AllRoles = []string{RoleAdmin, RoleDispatcher, RoleTechnician}

// Visit statuses (§3.1).
const (
	VisitStatusScheduled  = "scheduled"
	VisitStatusInProgress = "in_progress"
	VisitStatusCompleted  = "completed"
	VisitStatusCancelled  = "cancelled"
)

// AllVisitStatuses lists every status a visit may hold.
var AllVisitStatuses = []string{
	VisitStatusScheduled,
	VisitStatusInProgress,
	VisitStatusCompleted,
	VisitStatusCancelled,
}

// Visit priorities (§3.7).
const (
	VisitPriorityLow    = "low"
	VisitPriorityNormal = "normal"
	VisitPriorityHigh   = "high"
)

// AllVisitPriorities lists every priority a visit may hold, lowest first.
var AllVisitPriorities = []string{VisitPriorityLow, VisitPriorityNormal, VisitPriorityHigh}

// Clock event kinds (§4).
const (
	ClockKindIn  = "in"
	ClockKindOut = "out"
)

// Audit actions (§6.1). Combined with a resource type they name what happened.
const (
	AuditActionCreate   = "create"
	AuditActionUpdate   = "update"
	AuditActionDelete   = "delete"
	AuditActionCancel   = "cancel"
	AuditActionClockIn  = "clock_in"
	AuditActionClockOut = "clock_out"
	AuditActionLogin    = "login"
)

// Audit resource types.
const (
	ResourceUser  = "user"
	ResourceSite  = "site"
	ResourceVisit = "visit"
)

// IsValidRole reports whether role is one of the defined roles.
func IsValidRole(role string) bool {
	return contains(AllRoles, role)
}

// IsValidVisitStatus reports whether status is one of the defined visit statuses.
func IsValidVisitStatus(status string) bool {
	return contains(AllVisitStatuses, status)
}

// IsValidVisitPriority reports whether priority is one of the defined visit priorities.
func IsValidVisitPriority(priority string) bool {
	return contains(AllVisitPriorities, priority)
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nwasiq/fieldops/backend/internal/middleware"
	"github.com/nwasiq/fieldops/backend/internal/models"
	"github.com/nwasiq/fieldops/backend/internal/services"
)

type visitRequest struct {
	SiteID         uint   `json:"site_id"`
	TechnicianID   *uint  `json:"technician_id"`
	ScheduledStart string `json:"scheduled_start"`
	ScheduledEnd   string `json:"scheduled_end"`
	Priority       string `json:"priority"`
}

func (r visitRequest) input() (services.VisitInput, error) {
	start, err := parseInstant(r.ScheduledStart)
	if err != nil {
		return services.VisitInput{}, err
	}
	end, err := parseInstant(r.ScheduledEnd)
	if err != nil {
		return services.VisitInput{}, err
	}
	return services.VisitInput{SiteID: r.SiteID, TechnicianID: r.TechnicianID, ScheduledStart: start, ScheduledEnd: end, Priority: r.Priority}, nil
}

// parseInstant accepts RFC3339 only. A bare date or a wall-clock time without
// an offset has no instant to store, and guessing one is how a visit lands on
// the wrong day.
func parseInstant(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}

// ListVisits pages through visits in an inclusive run of Europe/London days
// (§3.2). Technicians are limited to their own visits by the service.
func (h *Handlers) ListVisits(c *gin.Context) {
	filter := services.VisitFilter{From: c.Query("from"), To: c.Query("to"), Status: c.Query("status"), Priority: c.Query("priority")}
	if raw := c.Query("technician_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			respondError(c, http.StatusBadRequest, "technician_id must be an integer")
			return
		}
		tech := uint(id)
		filter.TechnicianID = &tech
	}
	result, err := h.visits.List(c.Request.Context(), actorFrom(c), filter, pageFromQuery(c))
	if err != nil {
		fail(c, h.logger, err)
		return
	}
	visits := make([]visitView, 0, len(result.Items))
	for i := range result.Items {
		visits = append(visits, newVisitView(&result.Items[i]))
	}
	respond(c, http.StatusOK, visitListView{Visits: visits, pageView: newPageView(result)})
}

// GetVisit returns one visit with its clock events.
func (h *Handlers) GetVisit(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	visit, err := h.visits.Get(c.Request.Context(), actorFrom(c), id)
	if err != nil {
		fail(c, h.logger, err)
		return
	}
	respond(c, http.StatusOK, newVisitView(visit))
}

// CreateVisit schedules a visit.
func (h *Handlers) CreateVisit(c *gin.Context) {
	var req visitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	in, err := req.input()
	if err != nil {
		respondError(c, http.StatusBadRequest, "scheduled_start and scheduled_end must be RFC3339 timestamps")
		return
	}
	visit, err := h.visits.Create(c.Request.Context(), in)
	if err != nil {
		fail(c, h.logger, err)
		return
	}
	middleware.SetAuditResource(c, visit.ID)
	respond(c, http.StatusCreated, newVisitView(visit))
}

// UpdateVisit replaces the schedulable fields of an open visit.
func (h *Handlers) UpdateVisit(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	var req visitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	in, err := req.input()
	if err != nil {
		respondError(c, http.StatusBadRequest, "scheduled_start and scheduled_end must be RFC3339 timestamps")
		return
	}
	visit, err := h.visits.Update(c.Request.Context(), id, in)
	if err != nil {
		fail(c, h.logger, err)
		return
	}
	respond(c, http.StatusOK, newVisitView(visit))
}

// CancelVisit cancels an open visit (§3.4).
func (h *Handlers) CancelVisit(c *gin.Context) {
	h.transition(c, h.visits.Cancel)
}

// ClockIn records a clock-in (§4).
func (h *Handlers) ClockIn(c *gin.Context) {
	h.transition(c, h.visits.ClockIn)
}

// ClockOut records a clock-out (§4).
func (h *Handlers) ClockOut(c *gin.Context) {
	h.transition(c, h.visits.ClockOut)
}

type visitTransition func(ctx context.Context, id uint, actor services.Actor) (*models.Visit, error)

// transition is the single path for the three status moves: parse the id,
// apply the move as the caller, render the resulting visit.
func (h *Handlers) transition(c *gin.Context, move visitTransition) {
	id, err := pathID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	visit, err := move(c.Request.Context(), id, actorFrom(c))
	if err != nil {
		fail(c, h.logger, err)
		return
	}
	respond(c, http.StatusOK, newVisitView(visit))
}

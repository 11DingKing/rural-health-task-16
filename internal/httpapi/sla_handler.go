package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
	"ruralhealth/internal/sla"
)

// ListSLABreaches returns paginated SLA records filtered by escalation.
func (h *Handlers) ListSLABreaches(w http.ResponseWriter, r *http.Request) {
	if h.SLA == nil {
		writeError(w, errorsx.Wrap(errorsx.ErrNotFound, "sla service not configured"))
		return
	}
	page, pageSize := parsePagination(r)
	filter := r.URL.Query().Get("escalation")
	records, total, err := h.SLA.ListBreaches(r.Context(), domain.PaginationParams{Page: page, PageSize: pageSize}, filter)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]any, len(records))
	for i, rec := range records {
		items[i] = rec
	}
	writePaginated(w, items, total, page, pageSize)
}

// GetSLARecord returns the SLA record for a dispatch.
func (h *Handlers) GetSLARecord(w http.ResponseWriter, r *http.Request) {
	if h.SLA == nil {
		writeError(w, errorsx.Wrap(errorsx.ErrNotFound, "sla service not configured"))
		return
	}
	dispatchID := chi.URLParam(r, "dispatchID")
	rec, ok, err := h.SLA.GetByDispatch(r.Context(), dispatchID)
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		writeError(w, errorsx.ErrNotFound)
		return
	}
	writeOK(w, map[string]any{"record": rec})
}

// EscalateSLA manually escalates a breached SLA record.
func (h *Handlers) EscalateSLA(w http.ResponseWriter, r *http.Request) {
	if h.SLA == nil {
		writeError(w, errorsx.Wrap(errorsx.ErrNotFound, "sla service not configured"))
		return
	}
	dispatchID := chi.URLParam(r, "dispatchID")
	rec, err := h.SLA.Escalate(r.Context(), dispatchID)
	if err != nil {
		writeError(w, err)
		return
	}
	if h.Audit != nil {
		h.Audit.Record(r.Context(), "sla", rec.ID, "escalate", "operator", dispatchID)
	}
	writeOK(w, map[string]any{"record": rec})
}

// ResolveSLA marks an SLA record as resolved (terminal).
func (h *Handlers) ResolveSLA(w http.ResponseWriter, r *http.Request) {
	if h.SLA == nil {
		writeError(w, errorsx.Wrap(errorsx.ErrNotFound, "sla service not configured"))
		return
	}
	dispatchID := chi.URLParam(r, "dispatchID")
	if err := h.SLA.MarkResolved(r.Context(), dispatchID); err != nil {
		writeError(w, err)
		return
	}
	writeOK(w, map[string]string{"status": "resolved", "dispatch_id": dispatchID})
}

var _ = sla.Policy{}

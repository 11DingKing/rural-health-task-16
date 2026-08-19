package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
	"ruralhealth/internal/quota"
)

// GetQuotaUsage returns the active-submission quota usage for one enterprise.
func (h *Handlers) GetQuotaUsage(w http.ResponseWriter, r *http.Request) {
	if h.Quota == nil {
		writeError(w, errorsx.Wrap(errorsx.ErrNotFound, "quota service not configured"))
		return
	}
	enterpriseID := chi.URLParam(r, "enterpriseID")
	usage, err := h.Quota.GetUsage(r.Context(), enterpriseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeOK(w, map[string]any{"usage": usage})
}

// ListQuotaUsage returns paginated per-enterprise quota usage.
func (h *Handlers) ListQuotaUsage(w http.ResponseWriter, r *http.Request) {
	if h.Quota == nil {
		writeError(w, errorsx.Wrap(errorsx.ErrNotFound, "quota service not configured"))
		return
	}
	page, pageSize := parsePagination(r)
	usage, total, err := h.Quota.ListUsage(r.Context(), domain.PaginationParams{Page: page, PageSize: pageSize})
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]any, len(usage))
	for i, u := range usage {
		items[i] = u
	}
	writePaginated(w, items, total, page, pageSize)
}

// AcquireQuota reserves a slot for an enterprise. Used by operators and
// integration tests; the normal path acquires inside CreateSubmission.
func (h *Handlers) AcquireQuota(w http.ResponseWriter, r *http.Request) {
	if h.Quota == nil {
		writeError(w, errorsx.Wrap(errorsx.ErrNotFound, "quota service not configured"))
		return
	}
	enterpriseID := chi.URLParam(r, "enterpriseID")
	res, err := h.Quota.Acquire(r.Context(), enterpriseID)
	if err != nil {
		writeError(w, err)
		return
	}
	if h.Audit != nil {
		h.Audit.Record(r.Context(), "quota", res.ID, "acquire", enterpriseID, "manual acquire")
	}
	writeCreated(w, map[string]any{"reservation": res})
}

// ReleaseQuota releases a reservation by id.
func (h *Handlers) ReleaseQuota(w http.ResponseWriter, r *http.Request) {
	if h.Quota == nil {
		writeError(w, errorsx.Wrap(errorsx.ErrNotFound, "quota service not configured"))
		return
	}
	reservationID := chi.URLParam(r, "id")
	if err := h.Quota.Release(r.Context(), reservationID, "manual_release"); err != nil {
		writeError(w, err)
		return
	}
	writeOK(w, map[string]string{"status": "released", "reservation_id": reservationID})
}

var _ = quota.Policy{}

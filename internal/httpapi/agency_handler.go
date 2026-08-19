package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
)

func (h *Handlers) ListAgencies(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	_ = status
	agencies := h.Gateway.Upstreams()
	items := make([]any, len(agencies))
	for i, up := range agencies {
		items[i] = map[string]any{
			"id":       up.ID,
			"name":     up.Name,
			"base_url": up.BaseURL,
			"priority": up.Priority,
		}
	}
	writeOK(w, map[string]any{"agencies": items, "count": len(items)})
}

func (h *Handlers) CreateAgency(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code                string   `json:"code"`
		Name                string   `json:"name"`
		ContactEmail        string   `json:"contact_email"`
		AccreditedStandards []string `json:"accredited_standards"`
		Priority            int      `json:"priority"`
		MaxConcurrent       int      `json:"max_concurrent"`
	}
	if err := parseJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	agency := &domain.Agency{
		ID:                  "agn_" + uuid.NewString(),
		Code:                body.Code,
		Name:                body.Name,
		ContactEmail:        body.ContactEmail,
		IsActive:            true,
		Priority:            body.Priority,
		AccreditedStandards: body.AccreditedStandards,
		MaxConcurrent:       body.MaxConcurrent,
	}
	h.Audit.Record(r.Context(), "agency", agency.ID, "create", "admin", "agency created")
	writeCreated(w, map[string]any{"agency": agency})
}

func (h *Handlers) GetAgency(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agencies := h.Gateway.Upstreams()
	for _, up := range agencies {
		if up.ID == id {
			writeOK(w, map[string]any{"agency": up})
			return
		}
	}
	writeError(w, errorsx.ErrNotFound)
}

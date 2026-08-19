package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (h *Handlers) GetCallback(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cb, err := h.Callback.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeOK(w, map[string]any{"callback": cb})
}

func (h *Handlers) ConfirmCallback(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cb, err := h.Callback.ConfirmCallback(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	h.Audit.Record(r.Context(), "callback", cb.ID, "confirm", "enterprise", "callback confirmed")
	writeOK(w, map[string]any{"callback": cb})
}

func (h *Handlers) ListCallbacks(w http.ResponseWriter, r *http.Request) {
	subID := r.URL.Query().Get("submission_id")
	var items []any
	if subID != "" {
		cbs, err := h.Callback.FindBySubmission(r.Context(), subID)
		if err != nil {
			writeError(w, err)
			return
		}
		for _, c := range cbs {
			items = append(items, c)
		}
	}
	writeOK(w, map[string]any{"callbacks": items, "count": len(items)})
}

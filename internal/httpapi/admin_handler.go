package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"ruralhealth/internal/audit"
)

func (h *Handlers) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	entityType := r.URL.Query().Get("entity_type")
	entityID := r.URL.Query().Get("entity_id")
	actor := r.URL.Query().Get("actor")

	var entries []audit.AuditEntry
	var total int
	var err error

	if entityType != "" && entityID != "" {
		entries, err = h.Audit.FindByEntity(r.Context(), entityType, entityID)
		total = len(entries)
	} else if actor != "" {
		entries, err = h.Audit.FindByActor(r.Context(), actor)
		total = len(entries)
	} else {
		entries, total, err = h.Audit.ListPaged(r.Context(), page, pageSize)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]any, len(entries))
	for i, e := range entries {
		items[i] = e
	}
	if entityType != "" && entityID != "" || actor != "" {
		writeOK(w, map[string]any{"logs": items, "count": len(items)})
		return
	}
	writePaginated(w, items, total, page, pageSize)
}

func (h *Handlers) GatewayStatus(w http.ResponseWriter, r *http.Request) {
	statuses := h.Gateway.Status()
	traces := h.Gateway.Traces().List(r.Context(), "", 50)
	writeOK(w, map[string]any{
		"upstreams":     statuses,
		"recent_traces": traces,
	})
}

func (h *Handlers) ResetCircuit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Gateway.ResetCircuit(id); err != nil {
		writeError(w, err)
		return
	}
	h.Audit.Record(r.Context(), "upstream", id, "reset_circuit", "admin", "circuit breaker reset")
	writeOK(w, map[string]any{"message": "circuit reset", "upstream_id": id})
}

func (h *Handlers) SchedulerStats(w http.ResponseWriter, r *http.Request) {
	stats := h.Scheduler.Stats()
	writeOK(w, map[string]any{"stats": stats, "running": h.Scheduler.IsRunning()})
}

func (h *Handlers) BacklogStats(w http.ResponseWriter, r *http.Request) {
	dispatchBacklog, _ := h.Dispatch.FindPendingRetry(r.Context())
	callbackBacklog := 0
	deadLetters := 0
	writeOK(w, map[string]any{
		"dispatch_backlog": len(dispatchBacklog),
		"callback_backlog": callbackBacklog,
		"dead_letters":     deadLetters,
	})
}

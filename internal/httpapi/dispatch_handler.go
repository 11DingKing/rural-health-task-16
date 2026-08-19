package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"ruralhealth/internal/domain"
)

func (h *Handlers) ListDispatches(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	agencyFilter := r.URL.Query().Get("agency")
	statusFilter := r.URL.Query().Get("status")
	params := domain.PaginationParams{Page: page, PageSize: pageSize}
	tasks, total, err := h.Dispatch.ListPaged(r.Context(), params, agencyFilter, statusFilter)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]any, len(tasks))
	for i, t := range tasks {
		items[i] = t
	}
	writePaginated(w, items, total, page, pageSize)
}

func (h *Handlers) GetDispatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	task, err := h.Dispatch.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeOK(w, map[string]any{"task": task})
}

func (h *Handlers) RetryDispatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.Dispatch.HandleTimeout(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeOK(w, map[string]any{"message": "retry triggered", "task_id": id})
}

func (h *Handlers) SubmitResult(w http.ResponseWriter, r *http.Request) {
	var result domain.TestResult
	if err := parseJSON(r, &result); err != nil {
		writeError(w, err)
		return
	}
	task, err := h.Dispatch.Get(r.Context(), result.DispatchID)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.Consistency.MarkStale(r.Context(), task.SubmissionID); err != nil {
		writeError(w, err)
		return
	}
	if _, err := h.Consistency.RecomputeSummary(r.Context(), task.SubmissionID); err != nil {
		writeError(w, err)
		return
	}
	writeOK(w, map[string]any{"result": result, "message": "result processed"})
}

func (h *Handlers) GetSummary(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "submissionID")
	summary, err := h.Consistency.GetSummary(r.Context(), subID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeOK(w, map[string]any{"summary": summary})
}

func (h *Handlers) ListSummaries(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	params := domain.PaginationParams{Page: page, PageSize: pageSize}
	summaries, total, err := h.Consistency.ListSummaries(r.Context(), params)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]any, len(summaries))
	for i, s := range summaries {
		items[i] = s
	}
	writePaginated(w, items, total, page, pageSize)
}

func (h *Handlers) CheckConsistency(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "submissionID")
	check, err := h.Consistency.CheckConsistency(r.Context(), subID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeOK(w, map[string]any{"check": check})
}

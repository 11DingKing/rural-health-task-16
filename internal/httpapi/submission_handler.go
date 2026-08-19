package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"ruralhealth/internal/accesspolicy"
	"ruralhealth/internal/audit"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
	"ruralhealth/internal/quota"
	"ruralhealth/internal/submission"
)

func (h *Handlers) CreateSubmission(w http.ResponseWriter, r *http.Request) {
	var req submission.CreateRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	ctx := r.Context()

	// Acquire a quota slot before creating. If creation fails or turns
	// out to be a duplicate replay, release the slot so the concurrent
	// limit is never leaked. This acquire-then-release-on-failure is the
	// rollback path for the quota subsystem.
	var reservation *quota.Reservation
	if h.Quota != nil {
		res, err := h.Quota.Acquire(ctx, req.EnterpriseID)
		if err != nil {
			writeError(w, err)
			return
		}
		reservation = res
	}
	releaseReservation := func(reason string) {
		if reservation != nil {
			_ = h.Quota.Release(ctx, reservation.ID, reason)
		}
	}

	sub, isDuplicate, err := h.Submission.Create(ctx, req)
	if err != nil {
		releaseReservation("create_failed")
		writeError(w, err)
		return
	}
	if isDuplicate {
		releaseReservation("duplicate_replay")
		if h.Audit != nil {
			h.Audit.Record(ctx, "submission", sub.ID, "duplicate_submission", req.EnterpriseID, "idempotent replay")
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"submission":   sub,
			"is_duplicate": true,
			"message":      "duplicate submission, returning original result",
		})
		return
	}
	if reservation != nil {
		if err := h.Quota.Bind(ctx, reservation.ID, sub.ID, sub.ModelNo); err != nil {
			h.Logger.Error(ctx, "quota bind failed", "submission", sub.ID, "error", err)
		}
	}
	if h.Audit != nil {
		h.Audit.Record(ctx, "submission", sub.ID, "create", req.EnterpriseID, "new submission created")
	}
	writeCreated(w, map[string]any{"submission": sub})
}

func (h *Handlers) GetSubmission(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sub, err := h.Submission.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeOK(w, map[string]any{"submission": sub})
}

func (h *Handlers) ListSubmissions(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	statusFilter := r.URL.Query().Get("status")
	enterpriseFilter := r.URL.Query().Get("enterprise")
	params := domain.PaginationParams{Page: page, PageSize: pageSize}
	subs, total, err := h.Submission.List(r.Context(), params, statusFilter, enterpriseFilter)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]any, len(subs))
	for i, s := range subs {
		items[i] = s
	}
	writePaginated(w, items, total, page, pageSize)
}

func (h *Handlers) UpdateSubmission(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		ModelName     *string           `json:"model_name"`
		RiskLevel     *domain.RiskLevel `json:"risk_level"`
		StandardCodes *[]string         `json:"standard_codes"`
		Notes         *string           `json:"notes"`
		UpdatedBy     string            `json:"updated_by"`
	}
	if err := parseJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	sub, err := h.Submission.Update(r.Context(), id, submission.UpdateRequest{
		ModelName:     body.ModelName,
		RiskLevel:     body.RiskLevel,
		StandardCodes: body.StandardCodes,
		Notes:         body.Notes,
		UpdatedBy:     body.UpdatedBy,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	if h.Access != nil {
		_ = h.Access.Authorize(r.Context(), accesspolicy.FromRequest(r), accesspolicy.ActionSubmissionUpdate, id)
	}
	if h.Audit != nil {
		h.Audit.Record(r.Context(), "submission", sub.ID, "update", body.UpdatedBy, "submission updated")
	}
	writeOK(w, map[string]any{"submission": sub})
}

func (h *Handlers) CancelSubmission(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	if h.Access != nil {
		if err := h.Access.Authorize(ctx, accesspolicy.FromRequest(r), accesspolicy.ActionSubmissionCancel, id); err != nil {
			writeError(w, err)
			return
		}
	}
	var body struct {
		CancelledBy string `json:"cancelled_by"`
	}
	_ = parseJSON(r, &body)
	if body.CancelledBy == "" {
		body.CancelledBy = "unknown"
	}
	sub, err := h.Submission.Cancel(ctx, id, body.CancelledBy)
	if err != nil {
		writeError(w, err)
		return
	}
	if h.Quota != nil {
		_ = h.Quota.ReleaseBySubmission(ctx, id, "cancelled")
	}
	if h.Audit != nil {
		h.Audit.Record(ctx, "submission", sub.ID, "cancel", body.CancelledBy, "submission cancelled")
	}
	writeOK(w, map[string]any{"submission": sub})
}

func (h *Handlers) DispatchSubmission(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	tasks, err := h.Dispatch.DispatchSubmission(ctx, id)
	if err != nil {
		writeError(w, err)
		return
	}
	// Establish an SLA deadline for every newly dispatched task. Ensure
	// is idempotent, so re-dispatching does not create duplicate records.
	if h.SLA != nil {
		for _, task := range tasks {
			if _, err := h.SLA.Ensure(ctx, task); err != nil {
				h.Logger.Error(ctx, "sla ensure failed", "dispatch", task.ID, "error", err)
			}
		}
	}
	if h.Audit != nil {
		h.Audit.Record(ctx, "submission", id, "dispatch", "system", "dispatched to agencies")
	}
	items := make([]any, len(tasks))
	for i, t := range tasks {
		items[i] = t
	}
	writeOK(w, map[string]any{"tasks": items, "count": len(tasks)})
}

func (h *Handlers) BatchDispatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SubmissionIDs []string `json:"submission_ids"`
	}
	if err := parseJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	ctx := r.Context()
	results := make([]any, 0, len(body.SubmissionIDs))
	for _, id := range body.SubmissionIDs {
		tasks, err := h.Dispatch.DispatchSubmission(ctx, id)
		if err != nil {
			results = append(results, map[string]any{
				"submission_id": id,
				"error":         err.Error(),
			})
			continue
		}
		if h.SLA != nil {
			for _, task := range tasks {
				if _, err := h.SLA.Ensure(ctx, task); err != nil {
					h.Logger.Error(ctx, "sla ensure failed", "dispatch", task.ID, "error", err)
				}
			}
		}
		results = append(results, map[string]any{
			"submission_id": id,
			"task_count":    len(tasks),
		})
	}
	writeOK(w, map[string]any{"results": results})
}

func (h *Handlers) ExportReconciliation(w http.ResponseWriter, r *http.Request) {
	agencyFilter := r.URL.Query().Get("agency")
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	rows, err := h.Submission.ExportReconciliation(r.Context(), agencyFilter, dateFrom, dateTo)
	if err != nil {
		writeError(w, err)
		return
	}
	writeOK(w, map[string]any{"rows": rows, "count": len(rows)})
}

func (h *Handlers) TransitionSubmission(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	if h.Access != nil {
		if err := h.Access.Authorize(ctx, accesspolicy.FromRequest(r), accesspolicy.ActionSubmissionTransition, id); err != nil {
			writeError(w, err)
			return
		}
	}
	var body struct {
		Target domain.SubmissionStatus `json:"target"`
		Actor  string                  `json:"actor"`
	}
	if err := parseJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	sub, err := h.Submission.TransitionStatus(ctx, id, body.Target, body.Actor)
	if err != nil {
		writeError(w, err)
		return
	}
	// Reaching a terminal state frees the quota slot.
	if h.Quota != nil && body.Target.IsTerminal() {
		_ = h.Quota.ReleaseBySubmission(ctx, id, "terminal:"+string(body.Target))
	}
	writeOK(w, map[string]any{"submission": sub})
}

var _ = errorsx.ErrNotFound
var _ = audit.AuditEntry{}

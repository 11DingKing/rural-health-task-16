package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"ruralhealth/internal/middleware"
)

// NewRouter builds the HTTP router with all API routes.
// Routes are grouped by business domain and use semantic paths.
func NewRouter(h *Handlers) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.CORS)
	r.Use(chimw.RealIP)
	r.Use(middleware.Logger(h.Logger))
	r.Use(middleware.Recovery(h.Logger))

	r.Get("/healthz", h.Healthz)
	r.Get("/readyz", h.Readyz)

	r.Route("/api/v1", func(r chi.Router) {
		// Submissions
		r.Post("/submissions", h.CreateSubmission)
		r.Get("/submissions", h.ListSubmissions)
		r.Post("/submissions/batch-dispatch", h.BatchDispatch)
		r.Get("/submissions/export", h.ExportReconciliation)
		r.Get("/submissions/{id}", h.GetSubmission)
		r.Put("/submissions/{id}", h.UpdateSubmission)
		r.Delete("/submissions/{id}/cancel", h.CancelSubmission)
		r.Post("/submissions/{id}/dispatch", h.DispatchSubmission)
		r.Put("/submissions/{id}/transition", h.TransitionSubmission)

		// Dispatch tasks
		r.Get("/dispatches", h.ListDispatches)
		r.Get("/dispatches/{id}", h.GetDispatch)
		r.Post("/dispatches/{id}/retry", h.RetryDispatch)

		// Results & summaries
		r.Post("/results", h.SubmitResult)
		r.Get("/summaries", h.ListSummaries)
		r.Get("/summaries/{submissionID}", h.GetSummary)
		r.Post("/summaries/{submissionID}/check", h.CheckConsistency)

		// Rules & standards
		r.Post("/standards", h.CreateStandard)
		r.Get("/standards", h.ListStandards)
		r.Post("/rules", h.CreateRule)
		r.Get("/rules", h.ListRules)
		r.Get("/rules/{id}", h.GetRule)
		r.Put("/rules/{id}/expression", h.UpdateRuleExpression)
		r.Post("/rules/{id}/trial", h.StartTrial)
		r.Post("/rules/{id}/trials/{trialID}/approve", h.ApproveTrial)
		r.Post("/rules/{id}/trials/{trialID}/reject", h.RejectTrial)
		r.Get("/rules/{id}/versions", h.ListRuleVersions)
		r.Get("/rules/{id}/trials", h.ListRuleTrials)

		// Agencies
		r.Get("/agencies", h.ListAgencies)
		r.Post("/agencies", h.CreateAgency)
		r.Get("/agencies/{id}", h.GetAgency)

		// Callbacks
		r.Get("/callbacks", h.ListCallbacks)
		r.Get("/callbacks/{id}", h.GetCallback)
		r.Post("/callbacks/{id}/confirm", h.ConfirmCallback)

		// Admin
		r.Get("/audit", h.ListAuditLogs)
		r.Get("/gateway/status", h.GatewayStatus)
		r.Post("/gateway/{id}/reset", h.ResetCircuit)
		r.Get("/scheduler/stats", h.SchedulerStats)
		r.Get("/backlog", h.BacklogStats)

		// Policy & authorization (inspection)
		r.Get("/policy/actions", h.PolicyActions)
		r.Get("/policy/effective", h.PolicyEffective)

		// Quota enforcement
		r.Get("/quota", h.ListQuotaUsage)
		r.Get("/quota/{enterpriseID}", h.GetQuotaUsage)
		r.Post("/quota/{enterpriseID}/acquire", h.AcquireQuota)
		r.Post("/quota/reservations/{id}/release", h.ReleaseQuota)

		// SLA deadlines & escalation
		r.Get("/sla/breaches", h.ListSLABreaches)
		r.Get("/sla/{dispatchID}", h.GetSLARecord)
		r.Post("/sla/{dispatchID}/escalate", h.EscalateSLA)
		r.Post("/sla/{dispatchID}/resolve", h.ResolveSLA)
	})

	// Serve frontend static files if available
	r.Handle("/*", http.FileServer(http.Dir("web/dist")))

	return r
}

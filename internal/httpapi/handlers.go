package httpapi

import (
	"net/http"

	"ruralhealth/internal/accesspolicy"
	"ruralhealth/internal/audit"
	"ruralhealth/internal/callback"
	"ruralhealth/internal/consistency"
	"ruralhealth/internal/dispatch"
	"ruralhealth/internal/gateway"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/quota"
	"ruralhealth/internal/rulemgmt"
	"ruralhealth/internal/scheduler"
	"ruralhealth/internal/sla"
	"ruralhealth/internal/submission"
)

// Handlers holds all HTTP handler dependencies. It groups the services
// so the router can access them without global state.
type Handlers struct {
	Submission  *submission.Service
	Dispatch    *dispatch.Service
	Consistency *consistency.Service
	Callback    *callback.Service
	RuleMgmt    *rulemgmt.Service
	Audit       *audit.Service
	Gateway     *gateway.Manager
	Scheduler   *scheduler.Scheduler
	Access      *accesspolicy.Engine `json:"-"`
	Quota       *quota.Service       `json:"-"`
	SLA         *sla.Service         `json:"-"`
	Logger      logging.Logger
	ReadyDeps   *ReadyDeps
}

type ReadyDeps struct {
	CheckStore      func() bool
	CheckAudit      func() bool
	CheckScheduler  func() bool
	CheckMigrations func() bool
}

func (rd *ReadyDeps) AllReady() bool {
	return rd.CheckStore() && rd.CheckAudit() && rd.CheckScheduler() && rd.CheckMigrations()
}

func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) Readyz(w http.ResponseWriter, r *http.Request) {
	if h.ReadyDeps == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"reason": "dependencies not configured",
		})
		return
	}
	problems := []string{}
	if !h.ReadyDeps.CheckStore() {
		problems = append(problems, "kv store")
	}
	if !h.ReadyDeps.CheckAudit() {
		problems = append(problems, "audit database")
	}
	if !h.ReadyDeps.CheckScheduler() {
		problems = append(problems, "background scheduler")
	}
	if !h.ReadyDeps.CheckMigrations() {
		problems = append(problems, "migrations incomplete")
	}
	if len(problems) > 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":  "not ready",
			"reasons": problems,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

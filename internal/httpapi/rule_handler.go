package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"ruralhealth/internal/domain"
)

func (h *Handlers) CreateStandard(w http.ResponseWriter, r *http.Request) {
	var std domain.Standard
	if err := parseJSON(r, &std); err != nil {
		writeError(w, err)
		return
	}
	result, err := h.RuleMgmt.CreateStandard(r.Context(), &std)
	if err != nil {
		writeError(w, err)
		return
	}
	writeCreated(w, map[string]any{"standard": result})
}

func (h *Handlers) ListStandards(w http.ResponseWriter, r *http.Request) {
	standards, err := h.RuleMgmt.ListStandards(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]any, len(standards))
	for i, s := range standards {
		items[i] = s
	}
	writeOK(w, map[string]any{"standards": items, "count": len(items)})
}

func (h *Handlers) CreateRule(w http.ResponseWriter, r *http.Request) {
	var rule domain.Rule
	if err := parseJSON(r, &rule); err != nil {
		writeError(w, err)
		return
	}
	result, err := h.RuleMgmt.CreateRule(r.Context(), &rule)
	if err != nil {
		writeError(w, err)
		return
	}
	h.Audit.Record(r.Context(), "rule", result.ID, "create", "auditor", "rule created")
	writeCreated(w, map[string]any{"rule": result})
}

func (h *Handlers) GetRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rule, err := h.RuleMgmt.GetRule(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeOK(w, map[string]any{"rule": rule})
}

func (h *Handlers) ListRules(w http.ResponseWriter, r *http.Request) {
	standardCode := r.URL.Query().Get("standard")
	rules, err := h.RuleMgmt.ListRules(r.Context(), standardCode)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]any, len(rules))
	for i, rl := range rules {
		items[i] = rl
	}
	writeOK(w, map[string]any{"rules": items, "count": len(items)})
}

func (h *Handlers) UpdateRuleExpression(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Expression string `json:"expression"`
	}
	if err := parseJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	rule, err := h.RuleMgmt.UpdateRuleExpression(r.Context(), id, body.Expression)
	if err != nil {
		writeError(w, err)
		return
	}
	writeOK(w, map[string]any{"rule": rule})
}

func (h *Handlers) StartTrial(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		ContextJSON string `json:"context_json"`
	}
	if err := parseJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	trial, err := h.RuleMgmt.StartTrial(r.Context(), id, body.ContextJSON)
	if err != nil {
		writeError(w, err)
		return
	}
	h.Audit.Record(r.Context(), "rule", id, "trial", "auditor", "trial calculation started")
	writeOK(w, map[string]any{"trial": trial})
}

func (h *Handlers) ApproveTrial(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		ReviewedBy string `json:"reviewed_by"`
	}
	if err := parseJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	rule, err := h.RuleMgmt.ApproveTrial(r.Context(), id, body.ReviewedBy)
	if err != nil {
		writeError(w, err)
		return
	}
	h.Audit.Record(r.Context(), "rule", rule.ID, "approve", body.ReviewedBy, "trial approved, rule activated")
	writeOK(w, map[string]any{"rule": rule})
}

func (h *Handlers) RejectTrial(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		ReviewedBy string `json:"reviewed_by"`
		Reason     string `json:"reason"`
	}
	if err := parseJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	trial, err := h.RuleMgmt.RejectTrial(r.Context(), id, body.ReviewedBy, body.Reason)
	if err != nil {
		writeError(w, err)
		return
	}
	h.Audit.Record(r.Context(), "rule", trial.RuleID, "reject_trial", body.ReviewedBy, "trial rejected")
	writeOK(w, map[string]any{"trial": trial})
}

func (h *Handlers) ListRuleVersions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	versions, err := h.RuleMgmt.FindVersionsByRule(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]any, len(versions))
	for i, v := range versions {
		items[i] = v
	}
	writeOK(w, map[string]any{"versions": items, "count": len(items)})
}

func (h *Handlers) ListRuleTrials(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	trials, err := h.RuleMgmt.FindTrialsByRule(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]any, len(trials))
	for i, t := range trials {
		items[i] = t
	}
	writeOK(w, map[string]any{"trials": items, "count": len(items)})
}

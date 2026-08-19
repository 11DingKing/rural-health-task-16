package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"ruralhealth/internal/accesspolicy"
)

// PolicyEffective reports the authorization decision the engine would
// make for a subject, action, and resource. It is an inspection endpoint
// used by operators to verify role gating without performing the action.
func (h *Handlers) PolicyEffective(w http.ResponseWriter, r *http.Request) {
	if h.Access == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "policy engine not configured",
		})
		return
	}
	q := r.URL.Query()
	role, ok := accesspolicy.ParseRole(q.Get("role"))
	if !ok {
		writeError(w, accesspolicy.Deny("BAD_ROLE", "role query param must be a known role").Err)
		return
	}
	subj := accesspolicy.Subject{
		ID:           q.Get("actor_id"),
		Role:         role,
		EnterpriseID: q.Get("enterprise_id"),
		AgencyID:     q.Get("agency_id"),
	}
	action := accesspolicy.Action(q.Get("action"))
	resourceID := chi.URLParam(r, "id")
	if resourceID == "" {
		resourceID = q.Get("resource")
	}
	d := h.Access.Evaluate(r.Context(), subj, action, resourceID)
	writeOK(w, map[string]any{
		"allowed":     d.Allowed,
		"reason":      d.Reason,
		"error_code":  d.ErrorCode,
		"subject":     subj,
		"action":      action,
		"resource_id": resourceID,
	})
}

// PolicyActions lists every action governed by the policy.
func (h *Handlers) PolicyActions(w http.ResponseWriter, r *http.Request) {
	if h.Access == nil {
		writeError(w, accesspolicy.Deny("NOT_CONFIGURED", "policy engine not configured").Err)
		return
	}
	actions := h.Access.ListActions()
	items := make([]any, 0, len(actions))
	for _, a := range actions {
		spec, _ := h.Access.EffectivePolicy(a)
		items = append(items, map[string]any{
			"action":            string(a),
			"admin_only":        spec.AdminOnly(),
			"require_ownership": spec.RequireOwnership(),
			"roles":             stringRoles(h.Access.RolesFor(a)),
		})
	}
	writeOK(w, map[string]any{"actions": items, "count": len(items)})
}

func stringRoles(roles []accesspolicy.Role) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		out = append(out, string(r))
	}
	return out
}

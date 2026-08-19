package accesspolicy

import (
	"net/http"
	"strings"
)

// Header constants used to carry an authenticated subject across the
// HTTP boundary. The gateway layer resolves these (e.g. from a signed
// token) and forwards them to internal services.
const (
	HeaderActorID      = "X-Actor-Id"
	HeaderActorRole    = "X-Actor-Role"
	HeaderEnterpriseID = "X-Enterprise-Id"
	HeaderAgencyID     = "X-Agency-Id"
)

// FromRequest builds a Subject from request headers. When no role is
// present the Subject is anonymous, which the engine treats as allowed
// so existing public endpoints keep working during the rollout.
func FromRequest(r *http.Request) Subject {
	if r == nil {
		return Subject{}
	}
	role := Role(strings.TrimSpace(r.Header.Get(HeaderActorRole)))
	if role == "" {
		return Subject{}
	}
	return Subject{
		ID:           strings.TrimSpace(r.Header.Get(HeaderActorID)),
		Role:         role,
		EnterpriseID: strings.TrimSpace(r.Header.Get(HeaderEnterpriseID)),
		AgencyID:     strings.TrimSpace(r.Header.Get(HeaderAgencyID)),
	}
}

// ParseRole validates and normalizes a role string.
func ParseRole(s string) (Role, bool) {
	switch Role(strings.TrimSpace(s)) {
	case RoleEnterprise, RoleAgency, RoleAuditor, RoleAdmin, RoleSystem:
		return Role(strings.TrimSpace(s)), true
	}
	return "", false
}

// RolesFor returns the permitted roles for an action under the engine's
// current policy, or nil when the action is unknown.
func (e *Engine) RolesFor(action Action) []Role {
	e.mu.RLock()
	defer e.mu.RUnlock()
	spec, ok := e.policy[action]
	if !ok {
		return nil
	}
	out := make([]Role, len(spec.roles))
	copy(out, spec.roles)
	return out
}

package accesspolicy

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
)

// Evaluate resolves a single authorization decision. The algorithm:
//  1. Anonymous callers (no resolved Subject) are allowed, preserving
//     backward compatibility for endpoints that do not yet attach a
//     subject. Real enforcement activates once a Subject is present.
//  2. Unknown actions are denied.
//  3. The actor's role must be permitted for the action.
//  4. Admin-only actions require RoleAdmin.
//  5. Ownership is verified against the referenced resource.
//  6. Frozen submissions may only be acted on by admins.
//
// Denials wrap errorsx.ErrForbidden so handlers can classify them with
// errors.Is and map to HTTP 403.
func (e *Engine) Evaluate(ctx context.Context, subj Subject, action Action, resourceID string) Decision {
	if subj.IsAnonymous() {
		return Allow()
	}

	e.mu.RLock()
	spec, ok := e.policy[action]
	e.mu.RUnlock()
	if !ok {
		return Deny("UNKNOWN_ACTION", fmt.Sprintf("action %q is not governed by policy", action))
	}

	if spec.adminOnly && subj.Role != RoleAdmin && subj.Role != RoleSystem {
		return Deny("ROLE", fmt.Sprintf("action %q requires admin role, got %q", action, subj.Role))
	}

	if !rolePermitted(subj.Role, spec.roles) {
		return Deny("ROLE", fmt.Sprintf("role %q may not perform %q", subj.Role, action))
	}

	if spec.requireOwnership {
		owned, d := e.checkOwnership(ctx, subj, spec, resourceID)
		if !owned {
			return d
		}
	}

	return Allow()
}

// rolePermitted returns true when the role is in the allowed set.
// RoleSystem is always permitted so internal background tasks bypass
// role gating while still being audited.
func rolePermitted(role Role, allowed []Role) bool {
	if role == RoleSystem {
		return true
	}
	for _, r := range allowed {
		if r == role {
			return true
		}
	}
	return false
}

// checkOwnership loads the referenced resource and verifies that the
// subject owns it. Frozen submissions are protected: only admins may
// act on them.
func (e *Engine) checkOwnership(ctx context.Context, subj Subject, spec ruleSpec, resourceID string) (bool, Decision) {
	switch spec.kind {
	case resourceSubmission:
		return e.checkSubmissionOwnership(ctx, subj, resourceID)
	case resourceDispatch:
		return e.checkDispatchOwnership(ctx, subj, resourceID)
	default:
		return true, Allow()
	}
}

func (e *Engine) checkSubmissionOwnership(ctx context.Context, subj Subject, id string) (bool, Decision) {
	if e.submissions == nil {
		return true, Allow()
	}
	if id == "" {
		return false, Deny("BAD_RESOURCE", "submission id required for ownership check")
	}
	sub, found, err := e.submissions.Get(ctx, id)
	if err != nil {
		return false, Deny("LOOKUP_FAILED", err.Error())
	}
	if !found {
		return false, Deny("NOT_FOUND", fmt.Sprintf("submission %q not found", id))
	}
	if sub.Status == domain.SubmissionFrozen && subj.Role != RoleAdmin && subj.Role != RoleSystem {
		return false, Deny("FROZEN", "model is frozen for review; only admin may act")
	}
	if subj.Role == RoleEnterprise {
		if sub.EnterpriseID != subj.EnterpriseID {
			return false, Deny("NOT_OWNER", "enterprise may only act on its own submissions")
		}
		return true, Allow()
	}
	if subj.Role == RoleAgency || subj.Role == RoleAuditor {
		return false, Deny("ROLE", "only the owning enterprise or admin may modify a submission")
	}
	return true, Allow()
}

func (e *Engine) checkDispatchOwnership(ctx context.Context, subj Subject, id string) (bool, Decision) {
	if e.dispatches == nil {
		return true, Allow()
	}
	if id == "" {
		return false, Deny("BAD_RESOURCE", "dispatch id required for ownership check")
	}
	task, found, err := e.dispatches.Get(ctx, id)
	if err != nil {
		return false, Deny("LOOKUP_FAILED", err.Error())
	}
	if !found {
		return false, Deny("NOT_FOUND", fmt.Sprintf("dispatch %q not found", id))
	}
	if subj.Role == RoleAgency {
		if task.AgencyID != subj.AgencyID {
			return false, Deny("NOT_OWNER", "agency may only act on dispatches assigned to it")
		}
		return true, Allow()
	}
	return true, Allow()
}

// EffectivePolicy returns the ruleSpec for an action, for the inspection
// endpoint. It never mutates state and is safe for concurrent reads.
func (e *Engine) EffectivePolicy(action Action) (ruleSpec, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	spec, ok := e.policy[action]
	return spec, ok
}

// ListActions returns every governed action in a stable order, used by
// the policy inspection endpoint.
func (e *Engine) ListActions() []Action {
	e.mu.RLock()
	defer e.mu.RUnlock()
	acts := make([]Action, 0, len(e.policy))
	for a := range e.policy {
		acts = append(acts, a)
	}
	sort.Slice(acts, func(i, j int) bool { return acts[i] < acts[j] })
	return acts
}

// IsForbidden classifies an error returned by Authorize.
func IsForbidden(err error) bool { return errors.Is(err, errorsx.ErrForbidden) }

// DecisionError unwraps a decision error to its wrapped sentinel for
// classification at the HTTP boundary.
func DecisionError(d Decision) error {
	if d.Err == nil {
		return nil
	}
	return fmt.Errorf("authorization denied: %w", d.Err)
}

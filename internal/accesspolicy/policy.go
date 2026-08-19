package accesspolicy

import (
	"context"
	"sync"
	"time"

	"ruralhealth/internal/clock"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
)

// Role identifies the actor class attempting an action.
type Role string

const (
	RoleEnterprise Role = "enterprise"
	RoleAgency     Role = "agency"
	RoleAuditor    Role = "auditor"
	RoleAdmin      Role = "admin"
	RoleSystem     Role = "system"
)

// Action is a semantic permission name. Each state-changing handler
// maps to exactly one Action so decisions are auditable.
type Action string

const (
	ActionSubmissionCreate       Action = "submission.create"
	ActionSubmissionCancel       Action = "submission.cancel"
	ActionSubmissionTransition   Action = "submission.transition"
	ActionSubmissionUpdate       Action = "submission.update"
	ActionDispatchRetry          Action = "dispatch.retry"
	ActionDispatchExecute        Action = "dispatch.execute"
	ActionResultSubmit           Action = "result.submit"
	ActionRuleApproveTrial       Action = "rule.approve_trial"
	ActionRuleRejectTrial        Action = "rule.reject_trial"
	ActionRuleUpdateExpression   Action = "rule.update_expression"
	ActionGatewayResetCircuit    Action = "gateway.reset_circuit"
	ActionConsistencyForceRepair Action = "consistency.force_repair"
	ActionCallbackConfirm        Action = "callback.confirm"
	ActionQuotaInspect           Action = "quota.inspect"
)

// Subject is the authenticated actor resolved from request context.
// An empty Subject (Role == "") means "no subject present"; the engine
// allows such calls to preserve backward-compatible public endpoints.
type Subject struct {
	ID           string `json:"id"`
	Role         Role   `json:"role"`
	EnterpriseID string `json:"enterprise_id,omitempty"`
	AgencyID     string `json:"agency_id,omitempty"`
}

// IsAnonymous returns true when no subject was resolved.
func (s Subject) IsAnonymous() bool { return s.Role == "" }

// Decision is the outcome of an authorization evaluation.
type Decision struct {
	Allowed   bool   `json:"allowed"`
	Reason    string `json:"reason,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
	Err       error  `json:"-"`
}

// Allow is a positive decision.
func Allow() Decision { return Decision{Allowed: true} }

// Deny builds a negative decision wrapping ErrForbidden so callers can
// classify it with errors.Is(err, errorsx.ErrForbidden).
func Deny(code, reason string) Decision {
	return Decision{
		Allowed:   false,
		ErrorCode: code,
		Reason:    reason,
		Err:       errorsx.Wrap(errorsx.ErrForbidden, "%s: %s", code, reason),
	}
}

// resourceKind identifies which entity an ownership check applies to.
type resourceKind string

const (
	resourceNone       resourceKind = ""
	resourceSubmission resourceKind = "submission"
	resourceDispatch   resourceKind = "dispatch"
)

// ruleSpec defines who may perform an Action and whether ownership
// of the referenced resource is required.
type ruleSpec struct {
	roles            []Role
	adminOnly        bool
	requireOwnership bool
	kind             resourceKind
}

// policyTable maps Actions to their ruleSpec.
type policyTable map[Action]ruleSpec

// defaultPolicy returns the production rule set. The rules encode the
// separation of concerns between enterprises, agencies, auditors, and
// administrators.
func defaultPolicy() policyTable {
	all := []Role{RoleEnterprise, RoleAgency, RoleAuditor, RoleAdmin}
	mutating := []Role{RoleEnterprise, RoleAgency, RoleAdmin}
	return policyTable{
		ActionSubmissionCreate:       {roles: all, requireOwnership: false, kind: resourceNone},
		ActionSubmissionCancel:       {roles: mutating, requireOwnership: true, kind: resourceSubmission},
		ActionSubmissionTransition:   {roles: mutating, requireOwnership: true, kind: resourceSubmission},
		ActionSubmissionUpdate:       {roles: mutating, requireOwnership: true, kind: resourceSubmission},
		ActionDispatchRetry:          {roles: []Role{RoleAgency, RoleAdmin}, requireOwnership: true, kind: resourceDispatch},
		ActionDispatchExecute:        {roles: []Role{RoleAgency, RoleAdmin}, requireOwnership: true, kind: resourceDispatch},
		ActionResultSubmit:           {roles: []Role{RoleAgency, RoleAdmin}, requireOwnership: true, kind: resourceDispatch},
		ActionRuleApproveTrial:       {roles: []Role{RoleAdmin}, adminOnly: true},
		ActionRuleRejectTrial:        {roles: []Role{RoleAdmin}, adminOnly: true},
		ActionRuleUpdateExpression:   {roles: []Role{RoleAdmin}, adminOnly: true},
		ActionGatewayResetCircuit:    {roles: []Role{RoleAdmin}, adminOnly: true},
		ActionConsistencyForceRepair: {roles: []Role{RoleAdmin}, adminOnly: true},
		ActionCallbackConfirm:        {roles: []Role{RoleAgency, RoleAdmin}, requireOwnership: true, kind: resourceDispatch},
		ActionQuotaInspect:           {roles: all, requireOwnership: false, kind: resourceNone},
	}
}

// SubmissionLookup resolves a submission by id for ownership checks.
// *repository.SubmissionRepo satisfies this structurally.
type SubmissionLookup interface {
	Get(ctx context.Context, id string) (*domain.Submission, bool, error)
}

// DispatchLookup resolves a dispatch task by id for ownership checks.
// *repository.DispatchRepo satisfies this structurally.
type DispatchLookup interface {
	Get(ctx context.Context, id string) (*domain.DispatchTask, bool, error)
}

// Engine evaluates authorization decisions against a rule table,
// resolving resource ownership through the injected lookups.
type Engine struct {
	policy      policyTable
	submissions SubmissionLookup
	dispatches  DispatchLookup
	clk         clock.Clock
	mu          sync.RWMutex
}

// NewEngine builds an Engine with the default policy and optional
// resource lookups. When a lookup is nil the corresponding ownership
// check is skipped (used in tests that only exercise role gating).
func NewEngine(submissions SubmissionLookup, dispatches DispatchLookup, clk clock.Clock) *Engine {
	if clk == nil {
		clk = clock.Real()
	}
	return &Engine{
		policy:      defaultPolicy(),
		submissions: submissions,
		dispatches:  dispatches,
		clk:         clk,
	}
}

// ReplacePolicy swaps the active rule table. Used by tests and future
// dynamic policy reloads. The write is guarded by the engine mutex.
func (e *Engine) ReplacePolicy(p policyTable) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policy = p
}

// now returns the current time through the injected clock.
func (e *Engine) now() time.Time { return e.clk.Now() }

// Known reports whether an Action is governed by the policy.
func (e *Engine) Known(a Action) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.policy[a]
	return ok
}

// Authorize returns nil when the action is allowed, otherwise the
// decision's wrapped error. This is the convenience form used by
// handlers that only care about the allow/deny outcome.
func (e *Engine) Authorize(ctx context.Context, subj Subject, action Action, resourceID string) error {
	d := e.Evaluate(ctx, subj, action, resourceID)
	if d.Allowed {
		return nil
	}
	return d.Err
}

// AdminOnly reports whether only administrators may perform the action.
func (r ruleSpec) AdminOnly() bool { return r.adminOnly }

// RequireOwnership reports whether the action needs resource ownership.
func (r ruleSpec) RequireOwnership() bool { return r.requireOwnership }

// Kind returns the resource kind the action operates on.
func (r ruleSpec) Kind() string { return string(r.kind) }

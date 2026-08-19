package accesspolicy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"ruralhealth/internal/clock"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
)

// fakeSubmissionLookup is an in-memory SubmissionLookup for tests.
type fakeSubmissionLookup struct {
	subs map[string]*domain.Submission
}

func (f *fakeSubmissionLookup) Get(_ context.Context, id string) (*domain.Submission, bool, error) {
	s, ok := f.subs[id]
	return s, ok, nil
}

// fakeDispatchLookup is an in-memory DispatchLookup for tests.
type fakeDispatchLookup struct {
	tasks map[string]*domain.DispatchTask
}

func (f *fakeDispatchLookup) Get(_ context.Context, id string) (*domain.DispatchTask, bool, error) {
	t, ok := f.tasks[id]
	return t, ok, nil
}

func newTestEngine() (*Engine, *fakeSubmissionLookup, *fakeDispatchLookup) {
	subs := &fakeSubmissionLookup{subs: map[string]*domain.Submission{}}
	tasks := &fakeDispatchLookup{tasks: map[string]*domain.DispatchTask{}}
	return NewEngine(subs, tasks, clock.Real()), subs, tasks
}

func makeSubmission(id, enterpriseID string, status domain.SubmissionStatus) *domain.Submission {
	return &domain.Submission{
		ID:           id,
		EnterpriseID: enterpriseID,
		ModelNo:      "M-" + id,
		BatchNo:      "B-" + id,
		Status:       status,
		BaseCharge:   decimal.NewFromInt(500),
	}
}

func makeDispatch(id, agencyID string) *domain.DispatchTask {
	return &domain.DispatchTask{
		ID:       id,
		AgencyID: agencyID,
		Status:   domain.DispatchAssigned,
	}
}

func TestPolicy_AnonymousAllowed(t *testing.T) {
	e, _, _ := newTestEngine()
	d := e.Evaluate(context.Background(), Subject{}, ActionSubmissionCancel, "sub_1")
	if !d.Allowed {
		t.Fatalf("anonymous caller should be allowed, got deny %s: %s", d.ErrorCode, d.Reason)
	}
}

func TestPolicy_UnknownActionDenied(t *testing.T) {
	e, _, _ := newTestEngine()
	subj := Subject{ID: "u1", Role: RoleAdmin}
	d := e.Evaluate(context.Background(), subj, Action("bogus.action"), "")
	if d.Allowed {
		t.Fatal("unknown action should be denied")
	}
	if !IsForbidden(d.Err) {
		t.Fatalf("deny should wrap ErrForbidden, got %v", d.Err)
	}
}

func TestPolicy_AdminOnlyActionRejectedForEnterprise(t *testing.T) {
	e, _, _ := newTestEngine()
	subj := Subject{ID: "u1", Role: RoleEnterprise}
	err := e.Authorize(context.Background(), subj, ActionRuleApproveTrial, "rule_1")
	if err == nil {
		t.Fatal("enterprise must not approve trials")
	}
	if !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("expected ErrForbidden chain, got %v", err)
	}
}

func TestPolicy_AdminMayApproveTrial(t *testing.T) {
	e, _, _ := newTestEngine()
	subj := Subject{ID: "admin", Role: RoleAdmin}
	if err := e.Authorize(context.Background(), subj, ActionRuleApproveTrial, "rule_1"); err != nil {
		t.Fatalf("admin should approve trials, got %v", err)
	}
}

func TestPolicy_EnterpriseOwnershipEnforced(t *testing.T) {
	e, subs, _ := newTestEngine()
	subs.subs["sub_1"] = makeSubmission("sub_1", "ent_A", domain.SubmissionPending)

	owner := Subject{ID: "u", Role: RoleEnterprise, EnterpriseID: "ent_A"}
	if err := e.Authorize(context.Background(), owner, ActionSubmissionCancel, "sub_1"); err != nil {
		t.Fatalf("owner should be allowed: %v", err)
	}

	other := Subject{ID: "u", Role: RoleEnterprise, EnterpriseID: "ent_B"}
	err := e.Authorize(context.Background(), other, ActionSubmissionCancel, "sub_1")
	if err == nil || !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("non-owner should be forbidden, got %v", err)
	}
}

func TestPolicy_FrozenSubmissionProtected(t *testing.T) {
	e, subs, _ := newTestEngine()
	subs.subs["sub_2"] = makeSubmission("sub_2", "ent_A", domain.SubmissionFrozen)

	owner := Subject{ID: "u", Role: RoleEnterprise, EnterpriseID: "ent_A"}
	err := e.Authorize(context.Background(), owner, ActionSubmissionTransition, "sub_2")
	if err == nil || !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("owner must not act on frozen submission, got %v", err)
	}

	admin := Subject{ID: "a", Role: RoleAdmin}
	if err := e.Authorize(context.Background(), admin, ActionSubmissionTransition, "sub_2"); err != nil {
		t.Fatalf("admin may act on frozen submission, got %v", err)
	}
}

func TestPolicy_AgencyDispatchOwnership(t *testing.T) {
	e, _, tasks := newTestEngine()
	tasks.tasks["dsp_1"] = makeDispatch("dsp_1", "agency_A")

	own := Subject{ID: "a", Role: RoleAgency, AgencyID: "agency_A"}
	if err := e.Authorize(context.Background(), own, ActionResultSubmit, "dsp_1"); err != nil {
		t.Fatalf("owning agency should submit results: %v", err)
	}

	other := Subject{ID: "a", Role: RoleAgency, AgencyID: "agency_B"}
	err := e.Authorize(context.Background(), other, ActionResultSubmit, "dsp_1")
	if err == nil || !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("non-owning agency must be forbidden, got %v", err)
	}
}

func TestPolicy_AuditorCannotMutate(t *testing.T) {
	e, subs, _ := newTestEngine()
	subs.subs["sub_3"] = makeSubmission("sub_3", "ent_A", domain.SubmissionPending)
	auditor := Subject{ID: "au", Role: RoleAuditor}
	err := e.Authorize(context.Background(), auditor, ActionSubmissionCancel, "sub_3")
	if err == nil || !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("auditor must not mutate, got %v", err)
	}
}

func TestPolicy_NotFoundDenied(t *testing.T) {
	e, _, _ := newTestEngine()
	owner := Subject{ID: "u", Role: RoleEnterprise, EnterpriseID: "ent_A"}
	err := e.Authorize(context.Background(), owner, ActionSubmissionCancel, "missing")
	if err == nil || !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("missing resource should be forbidden, got %v", err)
	}
}

func TestPolicy_BadResourceDenied(t *testing.T) {
	e, _, _ := newTestEngine()
	owner := Subject{ID: "u", Role: RoleEnterprise, EnterpriseID: "ent_A"}
	err := e.Authorize(context.Background(), owner, ActionSubmissionCancel, "")
	if err == nil || !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("empty resource id should be forbidden, got %v", err)
	}
}

func TestPolicy_FromRequestAnonymous(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	if s := FromRequest(req); !s.IsAnonymous() {
		t.Fatal("request without role header should be anonymous")
	}
}

func TestPolicy_FromRequestBuildsSubject(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set(HeaderActorRole, "enterprise")
	req.Header.Set(HeaderActorID, "user-1")
	req.Header.Set(HeaderEnterpriseID, "ent_A")
	s := FromRequest(req)
	if s.IsAnonymous() || s.Role != RoleEnterprise || s.EnterpriseID != "ent_A" {
		t.Fatalf("unexpected subject %+v", s)
	}
}

func TestPolicy_ConcurrentEvaluateConsistent(t *testing.T) {
	e, subs, _ := newTestEngine()
	subs.subs["sub_9"] = makeSubmission("sub_9", "ent_A", domain.SubmissionPending)
	owner := Subject{ID: "u", Role: RoleEnterprise, EnterpriseID: "ent_A"}
	other := Subject{ID: "u", Role: RoleEnterprise, EnterpriseID: "ent_B"}

	var wg sync.WaitGroup
	const n = 50
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := e.Authorize(context.Background(), owner, ActionSubmissionCancel, "sub_9"); err != nil {
				t.Errorf("owner denied: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			err := e.Authorize(context.Background(), other, ActionSubmissionCancel, "sub_9")
			if err == nil || !errors.Is(err, errorsx.ErrForbidden) {
				t.Errorf("other should be forbidden, got %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestPolicy_ListActionsStable(t *testing.T) {
	e, _, _ := newTestEngine()
	acts := e.ListActions()
	if len(acts) == 0 {
		t.Fatal("expected governed actions")
	}
	for i := 1; i < len(acts); i++ {
		if acts[i-1] > acts[i] {
			t.Fatalf("actions not sorted: %v", acts)
		}
	}
}

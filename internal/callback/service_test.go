package callback

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ruralhealth/internal/config"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/persistence"
	"ruralhealth/internal/repository"
)

type testClock struct{ *clock.Mock }

func (t testClock) Now() time.Time                         { return t.Mock.Now() }
func (t testClock) NewTimer(d time.Duration) *time.Timer   { return time.NewTimer(d) }
func (t testClock) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }
func (t testClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (t testClock) Since(t2 time.Time) time.Duration       { return t.Mock.Now().Sub(t2) }

func newTestCallbackService(t *testing.T, webhookURL string) (*Service, *persistence.KVStore) {
	t.Helper()
	dir := t.TempDir()
	kv := persistence.NewKVStore(dir)
	require.NoError(t, kv.Open(context.Background()))
	t.Cleanup(func() { kv.Close() })

	store := repository.NewStore(kv)
	cbRepo := repository.NewCallbackRepo(store)
	subRepo := repository.NewSubmissionRepo(store)
	signer := NewSigner("test-secret-key")
	mockClock := clock.NewMock()
	clk := testClock{mockClock}
	cfg := config.CallbackConfig{MaxAttempts: 3, BaseBackoff: 10 * time.Millisecond, MaxBackoff: 100 * time.Millisecond, Timeout: 2 * time.Second}
	svc := NewService(cbRepo, subRepo, signer, logging.NewNop(), clk, cfg)
	_ = webhookURL
	return svc, kv
}

func TestCallback_SignerSignAndVerify(t *testing.T) {
	signer := NewSigner("secret")
	payload := []byte(`{"submission_id":"sub_1"}`)
	sig := signer.Sign(payload)
	assert.NotEmpty(t, sig)
	assert.True(t, signer.Verify(payload, sig), "signature should verify")
	assert.False(t, signer.Verify([]byte("tampered"), sig), "tampered payload should not verify")
}

func TestCallback_SignerBuildPayload(t *testing.T) {
	signer := NewSigner("secret")
	payload, sig, err := signer.BuildPayload("sub_1", map[string]any{"passed": true}, time.Now())
	require.NoError(t, err)
	assert.NotEmpty(t, payload)
	assert.NotEmpty(t, sig)
	assert.True(t, signer.Verify([]byte(payload), sig))
}

func TestCallback_CreateCallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc, _ := newTestCallbackService(t, srv.URL)
	cb, err := svc.CreateCallback(context.Background(), "sub_1", srv.URL, map[string]any{"passed": true})
	require.NoError(t, err)
	assert.NotEmpty(t, cb.ID)
	assert.Equal(t, domain.CallbackPending, cb.Status)
	assert.NotEmpty(t, cb.Signature)
}

func TestCallback_DuplicateCreateDedup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc, _ := newTestCallbackService(t, srv.URL)
	cb1, err := svc.CreateCallback(context.Background(), "sub_1", srv.URL, nil)
	require.NoError(t, err)

	cb2, err := svc.CreateCallback(context.Background(), "sub_1", srv.URL, nil)
	require.NoError(t, err)
	assert.Equal(t, cb1.ID, cb2.ID, "duplicate callback should return existing")
}

func TestCallback_DeliverSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.Header.Get("X-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc, _ := newTestCallbackService(t, srv.URL)
	cb, err := svc.CreateCallback(context.Background(), "sub_1", srv.URL, map[string]any{"passed": true})
	require.NoError(t, err)

	err = svc.DeliverCallback(context.Background(), cb.ID)
	require.NoError(t, err)

	updated, _, err := svc.callbackRepo.Get(context.Background(), cb.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.CallbackDelivered, updated.Status)
	assert.NotNil(t, updated.DeliveredAt)
}

func TestCallback_DeliverFailRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc, _ := newTestCallbackService(t, srv.URL)
	cb, err := svc.CreateCallback(context.Background(), "sub_1", srv.URL, nil)
	require.NoError(t, err)

	err = svc.DeliverCallback(context.Background(), cb.ID)
	assert.Error(t, err)

	updated, _, _ := svc.callbackRepo.Get(context.Background(), cb.ID)
	assert.Equal(t, domain.CallbackPending, updated.Status, "should be pending for retry")
	assert.NotNil(t, updated.NextRetryAt)
}

func TestCallback_ConfirmCallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc, _ := newTestCallbackService(t, srv.URL)
	cb, err := svc.CreateCallback(context.Background(), "sub_1", srv.URL, nil)
	require.NoError(t, err)

	err = svc.DeliverCallback(context.Background(), cb.ID)
	require.NoError(t, err)

	confirmed, err := svc.ConfirmCallback(context.Background(), cb.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.CallbackConfirmed, confirmed.Status)
	assert.NotNil(t, confirmed.ConfirmedAt)
}

func TestCallback_MaxAttemptsDeadLetter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	kv := persistence.NewKVStore(dir)
	require.NoError(t, kv.Open(context.Background()))
	defer kv.Close()
	store := repository.NewStore(kv)
	cbRepo := repository.NewCallbackRepo(store)
	subRepo := repository.NewSubmissionRepo(store)
	signer := NewSigner("key")
	mockClock := clock.NewMock()
	clk := testClock{mockClock}
	cfg := config.CallbackConfig{MaxAttempts: 2, BaseBackoff: 1 * time.Millisecond, MaxBackoff: 10 * time.Millisecond, Timeout: 2 * time.Second}
	svc := NewService(cbRepo, subRepo, signer, logging.NewNop(), clk, cfg)

	cb, err := svc.CreateCallback(context.Background(), "sub_1", srv.URL, nil)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		_ = svc.DeliverCallback(context.Background(), cb.ID)
	}

	updated, _, _ := cbRepo.Get(context.Background(), cb.ID)
	assert.Equal(t, domain.CallbackDead, updated.Status, "should be dead after max attempts")
}

func TestCallback_RetryPending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc, _ := newTestCallbackService(t, srv.URL)
	cb, err := svc.CreateCallback(context.Background(), "sub_1", srv.URL, nil)
	require.NoError(t, err)

	count, err := svc.RetryPending(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 1)

	updated, _, _ := svc.callbackRepo.Get(context.Background(), cb.ID)
	assert.Equal(t, domain.CallbackDelivered, updated.Status)
}

func TestCallback_GetNotFound(t *testing.T) {
	svc, _ := newTestCallbackService(t, "")
	_, err := svc.Get(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestCallback_InvalidTransitionReject(t *testing.T) {
	csm := CallbackStateMachine{}
	err := csm.Transition(domain.CallbackConfirmed, domain.CallbackDelivering)
	assert.Error(t, err)
	err = csm.Transition(domain.CallbackDelivered, domain.CallbackDead)
	assert.Error(t, err)
}

func TestCallback_PersistRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	kv := persistence.NewKVStore(dir)
	require.NoError(t, kv.Open(ctx))
	store := repository.NewStore(kv)
	cbRepo := repository.NewCallbackRepo(store)
	subRepo := repository.NewSubmissionRepo(store)
	signer := NewSigner("key")
	mockClock := clock.NewMock()
	clk := testClock{mockClock}
	cfg := config.CallbackConfig{MaxAttempts: 3, BaseBackoff: 10 * time.Millisecond, MaxBackoff: 100 * time.Millisecond, Timeout: 2 * time.Second}
	svc := NewService(cbRepo, subRepo, signer, logging.NewNop(), clk, cfg)

	cb, err := svc.CreateCallback(ctx, "sub_1", "http://example.com/hook", nil)
	require.NoError(t, err)
	require.NoError(t, kv.Close())

	kv2 := persistence.NewKVStore(dir)
	require.NoError(t, kv2.Open(ctx))
	defer kv2.Close()
	store2 := repository.NewStore(kv2)
	cbRepo2 := repository.NewCallbackRepo(store2)
	subRepo2 := repository.NewSubmissionRepo(store2)
	svc2 := NewService(cbRepo2, subRepo2, signer, logging.NewNop(), clk, cfg)

	recovered, err := svc2.Get(ctx, cb.ID)
	require.NoError(t, err)
	assert.Equal(t, cb.ID, recovered.ID)
	assert.Equal(t, cb.Status, recovered.Status)
}

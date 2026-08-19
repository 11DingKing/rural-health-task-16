package audit

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ruralhealth/internal/logging"
)

type auditClock struct{ *clock.Mock }

func (a auditClock) Now() time.Time                         { return a.Mock.Now() }
func (a auditClock) NewTimer(d time.Duration) *time.Timer   { return time.NewTimer(d) }
func (a auditClock) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }
func (a auditClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (a auditClock) Since(t time.Time) time.Duration        { return a.Mock.Now().Sub(t) }

func newTestAuditService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "audit.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	clk := auditClock{clock.NewMock()}
	return NewService(store, logging.NewNop(), clk)
}

func TestAudit_Record(t *testing.T) {
	svc := newTestAuditService(t)
	err := svc.Record(context.Background(), "submission", "sub_1", "create", "user_1", "new submission")
	require.NoError(t, err)
}

func TestAudit_FindByEntity(t *testing.T) {
	svc := newTestAuditService(t)
	ctx := context.Background()
	require.NoError(t, svc.Record(ctx, "submission", "sub_1", "create", "user_1", "created"))
	require.NoError(t, svc.Record(ctx, "submission", "sub_1", "update", "user_1", "updated"))
	require.NoError(t, svc.Record(ctx, "submission", "sub_2", "create", "user_1", "created"))

	entries, err := svc.FindByEntity(ctx, "submission", "sub_1")
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestAudit_FindByActor(t *testing.T) {
	svc := newTestAuditService(t)
	ctx := context.Background()
	require.NoError(t, svc.Record(ctx, "submission", "sub_1", "create", "user_a", ""))
	require.NoError(t, svc.Record(ctx, "submission", "sub_2", "create", "user_b", ""))
	require.NoError(t, svc.Record(ctx, "dispatch", "dsp_1", "retry", "user_a", ""))

	entries, err := svc.FindByActor(ctx, "user_a")
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestAudit_ListPaged(t *testing.T) {
	svc := newTestAuditService(t)
	ctx := context.Background()
	for i := 0; i < 25; i++ {
		require.NoError(t, svc.Record(ctx, "submission", "sub", "action", "user", ""))
	}

	entries, total, err := svc.ListPaged(ctx, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 25, total)
	assert.Len(t, entries, 10)

	entries, total, err = svc.ListPaged(ctx, 3, 10)
	require.NoError(t, err)
	assert.Equal(t, 25, total)
	assert.Len(t, entries, 5)
}

func TestAudit_PersistRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	store1, err := NewStore(filepath.Join(dir, "audit.db"))
	require.NoError(t, err)
	clk := auditClock{clock.NewMock()}
	svc1 := NewService(store1, logging.NewNop(), clk)
	require.NoError(t, svc1.Record(ctx, "submission", "sub_1", "create", "user_1", "before restart"))
	require.NoError(t, store1.Close())

	store2, err := NewStore(filepath.Join(dir, "audit.db"))
	require.NoError(t, err)
	defer store2.Close()
	svc2 := NewService(store2, logging.NewNop(), clk)

	entries, err := svc2.FindByEntity(ctx, "submission", "sub_1")
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "create", entries[0].Action)
}

func TestAudit_Ping(t *testing.T) {
	svc := newTestAuditService(t)
	err := svc.Ping(context.Background())
	assert.NoError(t, err)
}

func TestAudit_AuditContext(t *testing.T) {
	svc := newTestAuditService(t)
	ctx := context.Background()
	ac := svc.For("dispatch", "dsp_1", "system")
	err := ac.Action(ctx, "create", "dispatched submission")
	require.NoError(t, err)

	entries, err := svc.FindByEntity(ctx, "dispatch", "dsp_1")
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

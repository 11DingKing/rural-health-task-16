package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ruralhealth/internal/logging"
)

type testClock struct{ *clock.Mock }

func (t testClock) Now() time.Time                         { return t.Mock.Now() }
func (t testClock) NewTimer(d time.Duration) *time.Timer   { return time.NewTimer(d) }
func (t testClock) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }
func (t testClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (t testClock) Since(t2 time.Time) time.Duration       { return t.Mock.Now().Sub(t2) }

type mockTask struct {
	name     string
	interval time.Duration
	runCount *int32
	fail     bool
}

func (m *mockTask) Name() string            { return m.name }
func (m *mockTask) Interval() time.Duration { return m.interval }
func (m *mockTask) Run(ctx context.Context) error {
	atomic.AddInt32(m.runCount, 1)
	if m.fail {
		return errors.New("task failed")
	}
	return nil
}

func TestScheduler_StartStop(t *testing.T) {
	sched := New(logging.NewNop(), testClock{clock.NewMock()})
	require.False(t, sched.IsRunning())

	sched.Start(context.Background())
	assert.True(t, sched.IsRunning())

	sched.Stop(2 * time.Second)
	assert.False(t, sched.IsRunning())
}

func TestScheduler_TaskExecution(t *testing.T) {
	sched := New(logging.NewNop(), testClock{clock.NewMock()})
	var count int32
	task := &mockTask{name: "test", interval: 50 * time.Millisecond, runCount: &count}
	sched.Register(task)

	sched.Start(context.Background())
	time.Sleep(200 * time.Millisecond)
	sched.Stop(2 * time.Second)

	assert.GreaterOrEqual(t, atomic.LoadInt32(&count), int32(2), "task should have run at least twice")
}

func TestScheduler_TaskFailureTracking(t *testing.T) {
	sched := New(logging.NewNop(), testClock{clock.NewMock()})
	var count int32
	task := &mockTask{name: "failing", interval: 50 * time.Millisecond, runCount: &count, fail: true}
	sched.Register(task)

	sched.Start(context.Background())
	time.Sleep(200 * time.Millisecond)
	sched.Stop(2 * time.Second)

	stats := sched.Stats()
	taskStats, ok := stats["failing"]
	require.True(t, ok)
	assert.GreaterOrEqual(t, taskStats.FailCount, 1)
	assert.False(t, taskStats.LastRunOK)
	assert.NotEmpty(t, taskStats.LastError)
}

func TestScheduler_StatsCopy(t *testing.T) {
	sched := New(logging.NewNop(), testClock{clock.NewMock()})
	var count int32
	task := &mockTask{name: "t", interval: 50 * time.Millisecond, runCount: &count}
	sched.Register(task)

	sched.Start(context.Background())
	time.Sleep(100 * time.Millisecond)
	sched.Stop(2 * time.Second)

	stats := sched.Stats()
	s, ok := stats["t"]
	require.True(t, ok)
	assert.GreaterOrEqual(t, s.RunCount, 1)

	stats["t"].RunCount = 999
	original := sched.Stats()
	assert.NotEqual(t, 999, original["t"].RunCount, "stats should be a copy")
}

func TestScheduler_GracefulShutdown(t *testing.T) {
	sched := New(logging.NewNop(), testClock{clock.NewMock()})
	var count int32
	task := &mockTask{name: "slow", interval: 50 * time.Millisecond, runCount: &count}
	sched.Register(task)

	sched.Start(context.Background())
	time.Sleep(100 * time.Millisecond)
	sched.Stop(1 * time.Second)
	assert.False(t, sched.IsRunning())
}

func TestScheduler_MultipleTasks(t *testing.T) {
	sched := New(logging.NewNop(), testClock{clock.NewMock()})
	var c1, c2 int32
	sched.Register(&mockTask{name: "task1", interval: 50 * time.Millisecond, runCount: &c1})
	sched.Register(&mockTask{name: "task2", interval: 50 * time.Millisecond, runCount: &c2})

	sched.Start(context.Background())
	time.Sleep(200 * time.Millisecond)
	sched.Stop(2 * time.Second)

	assert.GreaterOrEqual(t, atomic.LoadInt32(&c1), int32(1))
	assert.GreaterOrEqual(t, atomic.LoadInt32(&c2), int32(1))
}

func TestScheduler_StartIdempotent(t *testing.T) {
	sched := New(logging.NewNop(), testClock{clock.NewMock()})
	var count int32
	sched.Register(&mockTask{name: "t", interval: 100 * time.Millisecond, runCount: &count})

	sched.Start(context.Background())
	sched.Start(context.Background()) // should not start twice
	time.Sleep(50 * time.Millisecond)
	sched.Stop(1 * time.Second)
}

func TestScheduler_StopWhenNotRunning(t *testing.T) {
	sched := New(logging.NewNop(), testClock{clock.NewMock()})
	sched.Stop(1 * time.Second) // should not panic
	assert.False(t, sched.IsRunning())
}

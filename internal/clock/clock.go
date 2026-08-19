package clock

import (
	"time"
)

// Clock abstracts time so tests can inject a deterministic clock.
type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) *time.Timer
	NewTicker(d time.Duration) *time.Ticker
	After(d time.Duration) <-chan time.Time
	Since(t time.Time) time.Duration
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) NewTimer(d time.Duration) *time.Timer   { return time.NewTimer(d) }
func (realClock) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (realClock) Since(t time.Time) time.Duration        { return time.Since(t) }

func Real() Clock { return realClock{} }

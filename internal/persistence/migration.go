package persistence

import (
	"time"
)

type timeNow = time.Time

var timeNowFunc = func() time.Time { return time.Now() }

// SetClock allows tests to inject a deterministic clock.
func SetClock(t func() time.Time) {
	timeNowFunc = t
}

// ResetClock restores the real clock.
func ResetClock() {
	timeNowFunc = time.Now
}

// MigrateIfNeeded runs data migrations based on the stored version.
// Currently only checks version compatibility; future versions can
// add real migration steps here.
func (s *KVStore) MigrateIfNeeded() error {
	if s.meta.Version == 0 {
		s.meta.Version = DataVersion
	}
	return nil
}

// RecordCount returns the approximate number of records in the store.
func (s *KVStore) RecordCount() int {
	return s.meta.RecordCount
}

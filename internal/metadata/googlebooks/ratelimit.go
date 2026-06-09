package googlebooks

import (
	"sync"
	"time"
)

// dailyLimiter caps Google Books API calls within any rolling 24-hour window so
// usage stays under the free quota (1,000 queries/project/day). A rolling window
// that admits at most N calls in any 24h guarantees at most N in any fixed
// calendar day, so it can't exceed Google's daily limit regardless of when
// Google's counter resets.
//
// State is in-memory, so a process restart clears the history — acceptable
// because the configured cap leaves headroom under 1,000 and restarts are rare.
type dailyLimiter struct {
	mu    sync.Mutex
	limit int
	calls []time.Time
	now   func() time.Time // overridable in tests
}

func newDailyLimiter(limit int) *dailyLimiter {
	return &dailyLimiter{limit: limit, now: time.Now}
}

// allow reports whether a call may proceed, recording it when so. A limit <= 0
// disables limiting (every call allowed). Calls older than 24h are evicted
// before the cap is checked.
func (l *dailyLimiter) allow() bool {
	if l == nil || l.limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := l.now().Add(-24 * time.Hour)
	drop := 0
	for drop < len(l.calls) && !l.calls[drop].After(cutoff) {
		drop++
	}
	l.calls = l.calls[drop:]

	if len(l.calls) >= l.limit {
		return false
	}
	l.calls = append(l.calls, l.now())
	return true
}

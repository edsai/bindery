package googlebooks

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestSearchBooks_OverDailyCapReturnsEmptyWithoutNetwork(t *testing.T) {
	calls := 0
	c := &Client{
		http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return mockResponse(t, http.StatusOK, volumeSearchResponse{}), nil
		})},
		limiter: newDailyLimiter(1),
	}
	if _, err := c.SearchBooks(context.Background(), "dune"); err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}
	// Over the cap: the merge must just get nothing — no error, no network call,
	// no spent quota.
	books, err := c.SearchBooks(context.Background(), "dune again")
	if err != nil {
		t.Errorf("over-cap call should return empty, not an error; got %v", err)
	}
	if len(books) != 0 {
		t.Errorf("over-cap call should return no books, got %d", len(books))
	}
	if calls != 1 {
		t.Errorf("over-cap call must NOT hit the network; transport calls = %d, want 1", calls)
	}
}

func TestDailyLimiter_AllowsUpToCap(t *testing.T) {
	l := newDailyLimiter(3)
	for i := 0; i < 3; i++ {
		if !l.allow() {
			t.Fatalf("call %d should be allowed (under cap 3)", i+1)
		}
	}
	if l.allow() {
		t.Error("4th call must be rejected once the cap is reached")
	}
}

func TestDailyLimiter_SlidingWindowExpiry(t *testing.T) {
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	l := newDailyLimiter(2)
	l.now = func() time.Time { return now }

	if !l.allow() || !l.allow() {
		t.Fatal("first two calls should be allowed")
	}
	if l.allow() {
		t.Fatal("third call should be rejected at the cap")
	}

	// Advance past 24h — the earlier calls fall out of the window.
	now = now.Add(25 * time.Hour)
	if !l.allow() {
		t.Error("after the 24h window expires, calls should be allowed again")
	}
}

func TestDailyLimiter_PartialWindowExpiry(t *testing.T) {
	base := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	cur := base
	l := newDailyLimiter(2)
	l.now = func() time.Time { return cur }

	l.allow() // t=0h
	cur = base.Add(23 * time.Hour)
	l.allow() // t=23h (window now full: [0h, 23h])
	if l.allow() {
		t.Fatal("at cap within the window")
	}
	cur = base.Add(24*time.Hour + time.Minute) // only the t=0h call has expired
	if !l.allow() {
		t.Error("one slot should have freed (t=0h call expired), call allowed")
	}
	if l.allow() {
		t.Error("the t=23h call is still in-window, so we should be back at cap")
	}
}

func TestDailyLimiter_ZeroCapDisables(t *testing.T) {
	l := newDailyLimiter(0)
	for i := 0; i < 5; i++ {
		if !l.allow() {
			t.Fatal("cap 0 means no limiting; all calls allowed")
		}
	}
}

package sampler

import (
	"context"
	"sync"
	"testing"
	"time"

	"market-feed/internal/model"
	"market-feed/internal/provider"
)

// fakeWriter captures inserted candles instead of hitting a database.
type fakeWriter struct {
	mu      sync.Mutex
	candles []model.PriceCandle
}

func (w *fakeWriter) Insert(c model.PriceCandle) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.candles = append(w.candles, c)
	return nil
}

func (w *fakeWriter) all() []model.PriceCandle {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]model.PriceCandle(nil), w.candles...)
}

func ev(base, quote string, price, vol float64, ts time.Time) provider.PriceEvent {
	return provider.PriceEvent{
		Base: base, Quote: quote, Price: price, Volume: vol,
		Timestamp: ts, Source: model.SourceBinance,
	}
}

// TestNewRejectsNonPredefinedInterval ensures a bucket size outside the closed
// set is rejected (so the caller can skip the job) instead of being coerced.
func TestNewRejectsNonPredefinedInterval(t *testing.T) {
	if _, err := New(90*time.Second, &fakeWriter{}); err == nil {
		t.Fatal("New should reject a non-predefined interval")
	}
	for _, d := range []time.Duration{time.Second, 5 * time.Second, time.Minute, 5 * time.Minute, time.Hour, 24 * time.Hour} {
		if _, err := New(d, &fakeWriter{}); err != nil {
			t.Errorf("New(%s) should be accepted: %v", d, err)
		}
	}
}

// TestAggregationOHLC checks that many trades in one interval collapse into a
// single candle with correct open/high/low/close and summed volume — the core
// promise that a 1m candle captures every intra-minute tick, not a snapshot.
func TestAggregationOHLC(t *testing.T) {
	w := &fakeWriter{}
	s, err := New(time.Minute, w)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	buckets := make(map[string]*bucket)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// open=100, then high=110, low=90, close=105; volumes sum to 6.0.
	s.add(buckets, ev("BTC", "USDT", 100, 1, base.Add(1*time.Second)))
	s.add(buckets, ev("BTC", "USDT", 110, 2, base.Add(10*time.Second)))
	s.add(buckets, ev("BTC", "USDT", 90, 2, base.Add(20*time.Second)))
	s.add(buckets, ev("BTC", "USDT", 105, 1, base.Add(30*time.Second)))

	s.flushAll(buckets)

	got := w.all()
	if len(got) != 1 {
		t.Fatalf("got %d candles, want 1", len(got))
	}
	c := got[0]
	switch {
	case c.Open != 100:
		t.Errorf("open = %v, want 100", c.Open)
	case c.High != 110:
		t.Errorf("high = %v, want 110", c.High)
	case c.Low != 90:
		t.Errorf("low = %v, want 90", c.Low)
	case c.Close != 105:
		t.Errorf("close = %v, want 105", c.Close)
	case c.Volume != 6:
		t.Errorf("volume = %v, want 6", c.Volume)
	}
	if c.Interval != model.Interval1m {
		t.Errorf("interval = %q, want 1m", c.Interval)
	}
	if !c.OpenTime.Equal(base) {
		t.Errorf("open_time = %v, want %v (truncated to minute)", c.OpenTime, base)
	}
}

// TestSeparateBucketsPerEdgeAndWindow verifies distinct (edge, window) pairs
// produce distinct candles.
func TestSeparateBucketsPerEdgeAndWindow(t *testing.T) {
	w := &fakeWriter{}
	s, err := New(time.Minute, w)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	buckets := make(map[string]*bucket)

	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Minute)
	s.add(buckets, ev("BTC", "USDT", 100, 1, t0))
	s.add(buckets, ev("BTC", "USDT", 101, 1, t1)) // next window
	s.add(buckets, ev("ETH", "USDT", 3, 1, t0))   // different edge, same window
	s.flushAll(buckets)

	if got := len(w.all()); got != 3 {
		t.Fatalf("got %d candles, want 3 (2 windows for BTC + 1 for ETH)", got)
	}
}

// TestRunFlushesOnChannelClose ensures open buckets are stored on shutdown
// rather than silently dropped.
func TestRunFlushesOnChannelClose(t *testing.T) {
	w := &fakeWriter{}
	s, err := New(time.Minute, w)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	in := make(chan provider.PriceEvent, 4)
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	in <- ev("BTC", "USDT", 100, 1, ts)
	in <- ev("BTC", "USDT", 120, 1, ts.Add(time.Second))
	close(in)

	done := make(chan struct{})
	go func() { s.Run(context.Background(), in); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after channel close")
	}

	got := w.all()
	if len(got) != 1 {
		t.Fatalf("got %d candles, want 1 flushed on close", len(got))
	}
	if got[0].High != 120 {
		t.Errorf("high = %v, want 120", got[0].High)
	}
}

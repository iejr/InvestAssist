package runner

import (
	"context"
	"sync"
	"testing"
	"time"

	"market-feed/internal/provider"
)

// recordingLatest captures every Upsert, guarded for the sink goroutine.
type recordingLatest struct {
	mu     sync.Mutex
	events []provider.PriceEvent
}

func (r *recordingLatest) Upsert(ev provider.PriceEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return nil
}

func (r *recordingLatest) snapshot() []provider.PriceEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]provider.PriceEvent(nil), r.events...)
}

func ev(base, quote string, price float64) provider.PriceEvent {
	return provider.PriceEvent{Base: base, Quote: quote, Price: price}
}

func TestRunLatest_NoCoalesceWritesEvery(t *testing.T) {
	in := make(chan provider.PriceEvent, 8)
	w := &recordingLatest{}
	done := make(chan struct{})
	go func() { runLatest(context.Background(), in, w, 0); close(done) }()

	in <- ev("BTC", "USDT", 1)
	in <- ev("BTC", "USDT", 2)
	in <- ev("BTC", "USDT", 3)
	close(in)
	<-done

	if got := len(w.snapshot()); got != 3 {
		t.Fatalf("no-coalesce should write every event, got %d want 3", got)
	}
}

func TestRunLatest_CoalesceKeepsNewestPerEdge(t *testing.T) {
	in := make(chan provider.PriceEvent, 8)
	w := &recordingLatest{}
	done := make(chan struct{})
	// Large window so nothing flushes until the channel closes; the final flush
	// then emits one write per edge with the newest value.
	go func() { runLatest(context.Background(), in, w, time.Hour); close(done) }()

	in <- ev("BTC", "USDT", 1)
	in <- ev("BTC", "USDT", 2)
	in <- ev("ETH", "USDT", 10)
	in <- ev("BTC", "USDT", 3) // newest for BTC
	close(in)
	<-done

	got := w.snapshot()
	if len(got) != 2 {
		t.Fatalf("coalesce should collapse to one per edge, got %d want 2", len(got))
	}
	prices := map[string]float64{}
	for _, e := range got {
		prices[e.Base] = e.Price
	}
	if prices["BTC"] != 3 || prices["ETH"] != 10 {
		t.Errorf("wrong coalesced values: %+v", prices)
	}
}

package bus

import (
	"testing"

	"market-feed/internal/provider"
)

func sample(price float64) provider.PriceEvent {
	return provider.PriceEvent{Base: "BTC", Quote: "USDT", Price: price}
}

// TestFanOut verifies every subscriber receives every published event.
func TestFanOut(t *testing.T) {
	b := New()
	a := b.Subscribe(4)
	c := b.Subscribe(4)

	b.Publish(sample(1))
	b.Publish(sample(2))
	b.Close()

	for _, sub := range []<-chan provider.PriceEvent{a, c} {
		var got []float64
		for ev := range sub {
			got = append(got, ev.Price)
		}
		if len(got) != 2 || got[0] != 1 || got[1] != 2 {
			t.Errorf("subscriber got %v, want [1 2]", got)
		}
	}
}

// TestDropOnFull verifies a full subscriber buffer drops events instead of
// blocking the publisher (ingestion must never stall on a slow consumer).
func TestDropOnFull(t *testing.T) {
	b := New()
	sub := b.Subscribe(1) // buffer of 1

	// Publish 3 without draining; only 1 fits, the rest are dropped. Publish
	// must not block.
	done := make(chan struct{})
	go func() {
		b.Publish(sample(1))
		b.Publish(sample(2))
		b.Publish(sample(3))
		close(done)
	}()

	<-done // if Publish blocked, this test would hang and time out.

	b.Close()
	count := 0
	for range sub {
		count++
	}
	if count != 1 {
		t.Errorf("received %d events, want 1 (rest dropped)", count)
	}
}

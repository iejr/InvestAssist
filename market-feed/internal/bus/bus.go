package bus

import (
	"sync"

	"market-feed/internal/provider"
)

// Bus fans out each PriceEvent to every subscriber. It is a simple in-process
// pub/sub: the ingestion goroutine publishes, and consumers (latest-price
// writer, sampler) each get their own buffered channel.
type Bus struct {
	mu   sync.RWMutex
	subs []chan provider.PriceEvent
}

func New() *Bus { return &Bus{} }

// Subscribe returns a new channel that receives every subsequently published
// event. The buffer absorbs bursts; if a slow consumer fills it, events are
// dropped for that subscriber rather than blocking the publisher.
func (b *Bus) Subscribe(buffer int) <-chan provider.PriceEvent {
	ch := make(chan provider.PriceEvent, buffer)
	b.mu.Lock()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
	return ch
}

// Publish delivers an event to all subscribers without blocking. If a
// subscriber's buffer is full the event is dropped for that subscriber.
func (b *Bus) Publish(ev provider.PriceEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs {
		select {
		case ch <- ev:
		default:
			// Subscriber is behind; drop rather than stall ingestion.
		}
	}
}

// Close closes all subscriber channels. Call once after publishing has stopped.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		close(ch)
	}
	b.subs = nil
}

package runner

import (
	"context"
	"log"
	"time"

	"market-feed/internal/bus"
	"market-feed/internal/model"
	"market-feed/internal/provider"
	"market-feed/internal/sampler"
)

// latestWriter upserts the newest value for an edge (satisfied by LatestRepo).
type latestWriter interface {
	Upsert(ev provider.PriceEvent) error
}

// candleWriter appends a sampled candle (satisfied by *repository.CandleRepo).
// Its method set matches sampler's own writer seam, so a value is assignable
// straight through to sampler.New without an adapter.
type candleWriter interface {
	Insert(c model.PriceCandle) error
}

// startSinks wires a bus to the shared sinks: one coalesced latest-writer and
// one sampler per interval. Both stream and poll sources publish to the same
// bus, so ingest is uniform — the sampler is frequency-agnostic, aggregating
// whatever lands in each window (a single poll observation per window becomes an
// open=high=low=close candle; empty windows produce no candle).
//
// The caller owns the bus (publishes events, and Close()s it on shutdown, which
// closes the sink channels and lets the goroutines drain and exit).
func startSinks(ctx context.Context, b *bus.Bus, latest latestWriter, candles candleWriter, intervals []time.Duration, coalesce time.Duration) {
	latestCh := b.Subscribe(1024)
	go runLatest(ctx, latestCh, latest, coalesce)

	for _, iv := range intervals {
		ch := b.Subscribe(1024)
		go sampler.New(iv, candles).Run(ctx, ch)
	}
}

// runLatest consumes events and upserts latest_prices. With coalesce<=0 it
// writes every event; otherwise it keeps only the newest event per edge and
// flushes at most once per coalesce window, so a bursty stream does not hammer
// Postgres. Any buffered writes are flushed when the channel closes or ctx ends.
func runLatest(ctx context.Context, in <-chan provider.PriceEvent, w latestWriter, coalesce time.Duration) {
	if coalesce <= 0 {
		for ev := range in {
			if err := w.Upsert(ev); err != nil {
				log.Printf("latest: upsert %s/%s: %v", ev.Base, ev.Quote, err)
			}
		}
		return
	}

	pending := make(map[string]provider.PriceEvent)
	flush := func() {
		for k, ev := range pending {
			if err := w.Upsert(ev); err != nil {
				log.Printf("latest: upsert %s/%s: %v", ev.Base, ev.Quote, err)
			}
			delete(pending, k)
		}
	}

	ticker := time.NewTicker(coalesce)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case ev, ok := <-in:
			if !ok {
				flush()
				return
			}
			pending[ev.Base+"/"+ev.Quote] = ev // newest per edge wins
		case <-ticker.C:
			flush()
		}
	}
}

package runner

import (
	"context"
	"log"
	"time"

	"market-feed/internal/bus"
	"market-feed/internal/provider"
	"market-feed/internal/sampler"
)

// latestWriter upserts the newest value for an edge (satisfied by LatestRepo).
type latestWriter interface {
	Upsert(ev provider.PriceEvent) error
}

// StreamRunner runs one streaming job: a single provider connection carrying N
// symbols, fanned out over a bus to (1) the latest-price writer and (2) the
// OHLC sampler. One connection, one bus, one latest-writer, one sampler per
// job — adding symbols does not add goroutines-per-symbol.
type StreamRunner struct {
	src            provider.Streamer
	symbols        []string
	latest         latestWriter
	sampler        *sampler.Sampler
	sampleInterval time.Duration
}

// NewStreamRunner builds a stream runner. sampleInterval sets the sampler's OHLC
// bucket size; candles are written through candleWriter (a *repository.CandleRepo).
func NewStreamRunner(src provider.Streamer, symbols []string, latest latestWriter, smp *sampler.Sampler, sampleInterval time.Duration) *StreamRunner {
	return &StreamRunner{
		src:            src,
		symbols:        symbols,
		latest:         latest,
		sampler:        smp,
		sampleInterval: sampleInterval,
	}
}

// Run streams until ctx is cancelled (or the provider stream closes). It blocks,
// so callers typically run it in its own goroutine.
func (r *StreamRunner) Run(ctx context.Context) error {
	events, err := r.src.Stream(ctx, r.symbols)
	if err != nil {
		return err
	}

	b := bus.New()
	latestCh := b.Subscribe(1024)
	sampleCh := b.Subscribe(1024)

	// Latest-price writer.
	go func() {
		for ev := range latestCh {
			if err := r.latest.Upsert(ev); err != nil {
				log.Printf("latest: upsert %s/%s: %v", ev.Base, ev.Quote, err)
			}
		}
	}()

	// OHLC sampler.
	go r.sampler.Run(ctx, sampleCh)

	log.Printf("stream: %v, sampling every %s", r.symbols, r.sampleInterval)

	// Pump provider events into the bus until the stream closes (ctx cancelled).
	for ev := range events {
		b.Publish(ev)
	}
	b.Close()
	return nil
}

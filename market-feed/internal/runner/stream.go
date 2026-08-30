package runner

import (
	"context"
	"log"
	"time"

	"market-feed/internal/bus"
	"market-feed/internal/provider"
)

// StreamRunner runs one streaming job: a single provider connection carrying N
// symbols, published to a bus and fanned out to the shared sinks (coalesced
// latest-writer + one sampler per interval). One connection, one bus, one
// latest-writer, N samplers per job — adding symbols does not add
// goroutines-per-symbol.
type StreamRunner struct {
	src       provider.Streamer
	symbols   []string
	latest    latestWriter
	candles   candleWriter
	intervals []time.Duration
	coalesce  time.Duration
}

// NewStreamRunner builds a stream runner. intervals are the sampler bucket sizes
// (one sampler each); coalesce bounds latest_prices write frequency per edge.
func NewStreamRunner(src provider.Streamer, symbols []string, latest latestWriter, candles candleWriter, intervals []time.Duration, coalesce time.Duration) *StreamRunner {
	return &StreamRunner{
		src:       src,
		symbols:   symbols,
		latest:    latest,
		candles:   candles,
		intervals: intervals,
		coalesce:  coalesce,
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
	startSinks(ctx, b, r.latest, r.candles, r.intervals, r.coalesce)

	log.Printf("stream: %v, sampling %v (latest coalesce %s)", r.symbols, r.intervals, r.coalesce)

	// Pump provider events into the bus until the stream closes (ctx cancelled).
	for ev := range events {
		b.Publish(ev)
	}
	b.Close()
	return nil
}

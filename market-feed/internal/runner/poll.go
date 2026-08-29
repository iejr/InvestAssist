package runner

import (
	"context"
	"fmt"
	"log"
	"time"

	"market-feed/internal/model"
	"market-feed/internal/provider"
)

// tickerSource observes a spot price for one edge. Satisfied by any
// provider.TickerFetcher; an interface so the runner is testable without a live
// exchange.
type tickerSource interface {
	Ticker(ctx context.Context, base, quote string) (float64, time.Time, error)
}

// PollRunner samples one edge's spot price on a slow timer and writes it to
// latest_prices. It is a self-clocked job, independent of the streaming bus.
//
// It writes only the latest row — deliberately NOT a candle. Historical OHLC for
// the same edge is the job of a separate backfill job (real candles from the
// provider), so the two never share write paths even when they share a driver.
type PollRunner struct {
	src         tickerSource
	latest      latestWriter
	base, quote string
	source      model.Source
	every       time.Duration
}

// NewPollRunner builds a poll runner for one (base, quote) edge.
func NewPollRunner(src tickerSource, latest latestWriter, base, quote string, source model.Source, every time.Duration) *PollRunner {
	return &PollRunner{src: src, latest: latest, base: base, quote: quote, source: source, every: every}
}

func (r *PollRunner) label() string { return r.base + "/" + r.quote }

// Run polls immediately, then every `every`, until ctx is cancelled. Polling on
// start means a restart refreshes the value right away rather than waiting a
// full interval. It blocks, so callers run it in its own goroutine.
func (r *PollRunner) Run(ctx context.Context) {
	log.Printf("poll: %s every %s", r.label(), r.every)
	r.pollOnce(ctx)

	ticker := time.NewTicker(r.every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.pollOnce(ctx)
		}
	}
}

// pollOnce observes the edge once and upserts the latest row. Errors are logged,
// not fatal — a transient provider hiccup must not kill the timer.
func (r *PollRunner) pollOnce(ctx context.Context) {
	if err := r.observe(ctx); err != nil {
		log.Printf("poll: %s: %v", r.label(), err)
	}
}

func (r *PollRunner) observe(ctx context.Context) error {
	price, ts, err := r.src.Ticker(ctx, r.base, r.quote)
	if err != nil {
		return err
	}
	ev := provider.PriceEvent{
		Base:      r.base,
		Quote:     r.quote,
		Price:     price,
		Timestamp: ts,
		Source:    r.source,
	}
	if err := r.latest.Upsert(ev); err != nil {
		return fmt.Errorf("latest upsert: %w", err)
	}
	log.Printf("poll: %s = %.6f", r.label(), price)
	return nil
}

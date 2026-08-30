package runner

import (
	"context"
	"log"
	"time"

	"market-feed/internal/bus"
	"market-feed/internal/model"
	"market-feed/internal/provider"
)

// tickerSource observes a spot price for one edge. Satisfied by any
// provider.TickerFetcher; an interface so the runner is testable without a live
// exchange.
type tickerSource interface {
	Ticker(ctx context.Context, base, quote string) (float64, time.Time, error)
}

// PollRunner samples one edge's spot price on a slow timer and publishes it to
// the same sink pipeline as a stream: the coalesced latest-writer keeps
// latest_prices fresh, and a single sampler at the poll cadence turns each
// observation into a candle. Because poll fires at most once per window, that
// candle is an honest open=high=low=close pass-through — real historical OHLC
// for the same edge is the separate backfill job's concern, and it overrides
// this bucket when it runs.
type PollRunner struct {
	src         tickerSource
	latest      latestWriter
	candles     candleWriter
	base, quote string
	source      model.Source
	every       time.Duration
}

// NewPollRunner builds a poll runner for one (base, quote) edge. The sampler
// interval equals `every` (one observation per window → pass-through candle).
func NewPollRunner(src tickerSource, latest latestWriter, candles candleWriter, base, quote string, source model.Source, every time.Duration) *PollRunner {
	return &PollRunner{src: src, latest: latest, candles: candles, base: base, quote: quote, source: source, every: every}
}

func (r *PollRunner) label() string { return r.base + "/" + r.quote }

// Run polls immediately, then every `every`, until ctx is cancelled. Polling on
// start means a restart refreshes the value right away rather than waiting a
// full interval. It blocks, so callers run it in its own goroutine.
func (r *PollRunner) Run(ctx context.Context) {
	b := bus.New()
	// coalesce=0: poll is already slow, so write each observation straight
	// through. The lone sampler at `every` produces one candle per window.
	startSinks(ctx, b, r.latest, r.candles, []time.Duration{r.every}, 0)
	defer b.Close()

	log.Printf("poll: %s every %s", r.label(), r.every)
	r.publish(ctx, b)

	ticker := time.NewTicker(r.every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.publish(ctx, b)
		}
	}
}

// publish observes the edge once and publishes it to the bus. Errors are logged,
// not fatal — a transient provider hiccup must not kill the timer.
func (r *PollRunner) publish(ctx context.Context, b *bus.Bus) {
	ev, ok := r.fetch(ctx)
	if !ok {
		return
	}
	b.Publish(ev)
	log.Printf("poll: %s = %.6f", r.label(), ev.Price)
}

// fetch observes the edge's spot price and builds a PriceEvent. ok is false (and
// the error is logged) when the source fails.
func (r *PollRunner) fetch(ctx context.Context) (provider.PriceEvent, bool) {
	price, ts, err := r.src.Ticker(ctx, r.base, r.quote)
	if err != nil {
		log.Printf("poll: %s: %v", r.label(), err)
		return provider.PriceEvent{}, false
	}
	return provider.PriceEvent{
		Base:      r.base,
		Quote:     r.quote,
		Price:     price,
		Timestamp: ts,
		Source:    r.source,
	}, true
}

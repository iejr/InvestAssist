package runner

import (
	"context"
	"time"

	"market-feed/internal/backfill"
	"market-feed/internal/model"
)

// BackfillRunner runs one backfill job over a rolling window ending at "now" and
// starting `lookback` earlier. It is one-shot: the recurring cadence is driven
// externally (cron invoking the -backfill CLI), keeping backfill distinct from
// the always-on stream/poll jobs.
type BackfillRunner struct {
	bf          *backfill.Backfiller
	base, quote string
	iv          model.Interval
	lookback    time.Duration
	override    bool
}

// NewBackfillRunner builds a backfill runner around a prepared Backfiller.
func NewBackfillRunner(bf *backfill.Backfiller, base, quote string, iv model.Interval, lookback time.Duration, override bool) *BackfillRunner {
	return &BackfillRunner{bf: bf, base: base, quote: quote, iv: iv, lookback: lookback, override: override}
}

// Run backfills [now-lookback, now) for the edge. now is passed in so callers
// control the clock (and tests are deterministic).
func (r *BackfillRunner) Run(ctx context.Context, now time.Time) error {
	start := now.Add(-r.lookback)
	return r.bf.Run(ctx, r.base, r.quote, r.iv, start, now, r.override)
}

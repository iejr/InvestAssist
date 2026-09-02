package runner

import (
	"context"
	"time"

	"market-feed/internal/backfill"
	"market-feed/internal/model"
)

// BackfillRunner runs one backfill job over a fully-resolved [start, end)
// window. It is one-shot: the recurring cadence is driven externally (cron
// invoking the -backfill CLI), keeping backfill distinct from the always-on
// stream/poll jobs. Window resolution (the -from/-to/-lookback vs. YAML
// precedence) is the caller's concern; the runner just executes.
type BackfillRunner struct {
	bf          *backfill.Backfiller
	base, quote string
	iv          model.Interval
	start, end  time.Time
	override    bool
}

// NewBackfillRunner builds a backfill runner around a prepared Backfiller and a
// resolved window.
func NewBackfillRunner(bf *backfill.Backfiller, base, quote string, iv model.Interval, start, end time.Time, override bool) *BackfillRunner {
	return &BackfillRunner{bf: bf, base: base, quote: quote, iv: iv, start: start, end: end, override: override}
}

// Run backfills the resolved window for the edge.
func (r *BackfillRunner) Run(ctx context.Context) error {
	return r.bf.Run(ctx, r.base, r.quote, r.iv, r.start, r.end, r.override)
}

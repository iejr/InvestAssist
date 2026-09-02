// Package backfill fetches historical candles from any OHLC-capable provider
// and stores them, either overwriting existing rows (override) or filling only
// missing buckets (gap-fill). It is provider-agnostic: it depends on the
// provider.OHLCFetcher capability, not on any specific exchange.
package backfill

import (
	"context"
	"fmt"
	"log"
	"time"

	"market-feed/internal/model"
	"market-feed/internal/provider"
	"market-feed/internal/repository"
)

// candleStore persists candles and reports what already exists.
// *repository.CandleRepo satisfies it.
type candleStore interface {
	InsertBatch(candles []model.PriceCandle) error
	UpsertBatch(candles []model.PriceCandle) error
	ExistingOpenTimes(base, quote string, iv model.Interval, source model.Source, start, end time.Time) (map[int64]struct{}, error)
}

// Backfiller coordinates fetching candles from an OHLC provider and writing
// them. The provider is any provider.OHLCFetcher (Binance, Kraken, ...).
type Backfiller struct {
	src   provider.OHLCFetcher
	store candleStore
}

func New(src provider.OHLCFetcher, store candleStore) *Backfiller {
	return &Backfiller{src: src, store: store}
}

// Run backfills [start, end) for one (base, quote) edge at one interval.
//
//   - end is clamped to the last CLOSED interval boundary so an in-progress
//     candle is never fetched or stored.
//   - override=true overwrites existing rows (authoritative candles win);
//     override=false fills only buckets not already present.
func (b *Backfiller) Run(ctx context.Context, base, quote string, iv model.Interval, start, end time.Time, override bool) error {
	dur, err := IntervalDuration(iv)
	if err != nil {
		return err
	}
	edge := base + "/" + quote

	// Clamp the upper bound to the last closed boundary.
	lastClosed := end.UTC().Truncate(dur)
	if !lastClosed.After(start) {
		log.Printf("backfill %s %s: nothing to do (range before first closed candle)", edge, iv)
		return nil
	}
	start = start.UTC().Truncate(dur)

	candles, err := b.src.FetchOHLC(ctx, base, quote, iv, start, lastClosed)
	if err != nil {
		return fmt.Errorf("fetch ohlc: %w", err)
	}
	if len(candles) == 0 {
		log.Printf("backfill %s %s: no candles returned for range %s..%s", edge, iv, start, lastClosed)
		return nil
	}

	if override {
		if err := b.store.UpsertBatch(candles); err != nil {
			return fmt.Errorf("upsert candles: %w", err)
		}
		log.Printf("backfill %s %s: wrote %d candles (override)", edge, iv, len(candles))
		return nil
	}

	// Gap-fill: keep only candles whose bucket isn't already stored. Source is
	// taken from the fetched candles so the existence check matches the provider
	// that produced them.
	existing, err := b.store.ExistingOpenTimes(base, quote, iv, candles[0].Source, start, lastClosed)
	if err != nil {
		return fmt.Errorf("read existing candles: %w", err)
	}
	missing := candles[:0:0]
	for _, c := range candles {
		if _, present := existing[c.OpenTime.UnixMilli()]; !present {
			missing = append(missing, c)
		}
	}
	if err := b.store.InsertBatch(missing); err != nil {
		return fmt.Errorf("insert candles: %w", err)
	}
	log.Printf("backfill %s %s: filled %d missing of %d candles (gap-fill)", edge, iv, len(missing), len(candles))
	return nil
}

// IntervalDuration returns the wall-clock duration of a stored interval.
func IntervalDuration(iv model.Interval) (time.Duration, error) {
	switch iv {
	case model.Interval1s:
		return time.Second, nil
	case model.Interval5s:
		return 5 * time.Second, nil
	case model.Interval1m:
		return time.Minute, nil
	case model.Interval5m:
		return 5 * time.Minute, nil
	case model.Interval1h:
		return time.Hour, nil
	case model.Interval1d:
		return 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown interval %q", iv)
	}
}

// compile-time check that the concrete store satisfies our seam. The OHLC
// source seam (provider.OHLCFetcher) is asserted in the provider package.
var _ candleStore = (*repository.CandleRepo)(nil)

// Package backfill fetches historical klines from Binance and stores them as
// candles, either overwriting existing rows (override) or filling only missing
// buckets (gap-fill).
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

// klineSource fetches historical candles. *provider.BinanceREST satisfies it;
// tests inject a fake.
type klineSource interface {
	Klines(ctx context.Context, symbol string, iv model.Interval, start, end time.Time) ([]model.PriceCandle, error)
}

// candleStore persists candles and reports what already exists.
// *repository.CandleRepo satisfies it.
type candleStore interface {
	InsertBatch(candles []model.PriceCandle) error
	UpsertBatch(candles []model.PriceCandle) error
	ExistingOpenTimes(base, quote string, iv model.Interval, source model.Source, start, end time.Time) (map[int64]struct{}, error)
}

// Backfiller coordinates fetching klines and writing candles.
type Backfiller struct {
	src   klineSource
	store candleStore
}

func New(src klineSource, store candleStore) *Backfiller {
	return &Backfiller{src: src, store: store}
}

// Run backfills [start, end) for one symbol at one interval.
//
//   - end is clamped to the last CLOSED interval boundary so an in-progress
//     candle is never fetched or stored.
//   - override=true overwrites existing rows (authoritative klines win);
//     override=false fills only buckets not already present.
func (b *Backfiller) Run(ctx context.Context, symbol string, iv model.Interval, start, end time.Time, override bool) error {
	dur, err := IntervalDuration(iv)
	if err != nil {
		return err
	}

	// Clamp the upper bound to the last closed boundary.
	lastClosed := end.UTC().Truncate(dur)
	if !lastClosed.After(start) {
		log.Printf("backfill %s %s: nothing to do (range before first closed candle)", symbol, iv)
		return nil
	}
	start = start.UTC().Truncate(dur)

	candles, err := b.src.Klines(ctx, symbol, iv, start, lastClosed)
	if err != nil {
		return fmt.Errorf("fetch klines: %w", err)
	}
	if len(candles) == 0 {
		log.Printf("backfill %s %s: no klines returned for range %s..%s", symbol, iv, start, lastClosed)
		return nil
	}

	if override {
		if err := b.store.UpsertBatch(candles); err != nil {
			return fmt.Errorf("upsert candles: %w", err)
		}
		log.Printf("backfill %s %s: wrote %d candles (override)", symbol, iv, len(candles))
		return nil
	}

	// Gap-fill: keep only candles whose bucket isn't already stored.
	base, quote := candles[0].Base, candles[0].Quote
	existing, err := b.store.ExistingOpenTimes(base, quote, iv, model.SourceBinance, start, lastClosed)
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
	log.Printf("backfill %s %s: filled %d missing of %d candles (gap-fill)", symbol, iv, len(missing), len(candles))
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

// compile-time checks that the concrete types satisfy our seams.
var (
	_ klineSource = (*provider.BinanceREST)(nil)
	_ candleStore = (*repository.CandleRepo)(nil)
)

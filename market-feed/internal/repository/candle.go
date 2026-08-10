package repository

import (
	"time"

	"market-feed/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CandleRepo appends OHLC observations. Writes are idempotent on the
// (base, quote, interval, open_time, source) unique key so a re-emitted or
// backfilled candle does not duplicate.
type CandleRepo struct {
	db *gorm.DB
}

func NewCandleRepo(db *gorm.DB) *CandleRepo { return &CandleRepo{db: db} }

// candleConflict is the unique key candles collide on.
var candleConflict = []clause.Column{
	{Name: "base"}, {Name: "quote"}, {Name: "interval"},
	{Name: "open_time"}, {Name: "source"},
}

// candleBatchSize bounds rows per INSERT so a large backfill is a handful of
// round-trips rather than one statement per row (matters for a remote DB).
const candleBatchSize = 500

// Insert appends a candle, ignoring the write if that bucket already exists.
// Used by the live sampler: first write wins (append-only).
func (r *CandleRepo) Insert(c model.PriceCandle) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   candleConflict,
		DoNothing: true,
	}).Create(&c).Error
}

// InsertBatch appends many candles, skipping any that already exist. Used by
// gap-fill backfill (override off) so existing rows are never touched.
func (r *CandleRepo) InsertBatch(candles []model.PriceCandle) error {
	if len(candles) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   candleConflict,
		DoNothing: true,
	}).CreateInBatches(candles, candleBatchSize).Error
}

// UpsertBatch writes many candles, overwriting the OHLCV of existing rows on
// conflict. Used by override backfill so authoritative klines replace any
// partial/live-sampled candle for the same bucket.
func (r *CandleRepo) UpsertBatch(candles []model.PriceCandle) error {
	if len(candles) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   candleConflict,
		DoUpdates: clause.AssignmentColumns([]string{"open", "high", "low", "close", "volume", "created_at"}),
	}).CreateInBatches(candles, candleBatchSize).Error
}

// ExistingOpenTimes returns the set of open_time values already stored for one
// (base, quote, interval, source) within [start, end). Used by the gap planner
// to compute which buckets are missing without a database-specific query.
func (r *CandleRepo) ExistingOpenTimes(base, quote string, iv model.Interval, source model.Source, start, end time.Time) (map[int64]struct{}, error) {
	var times []time.Time
	err := r.db.Model(&model.PriceCandle{}).
		Where("base = ? AND quote = ? AND interval = ? AND source = ? AND open_time >= ? AND open_time < ?",
			base, quote, iv, source, start, end).
		Order("open_time asc").
		Pluck("open_time", &times).Error
	if err != nil {
		return nil, err
	}
	set := make(map[int64]struct{}, len(times))
	for _, t := range times {
		set[t.UTC().UnixMilli()] = struct{}{}
	}
	return set, nil
}

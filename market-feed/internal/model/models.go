package model

import "time"

// Interval is the OHLC bucket size for historical candles. The value set is
// closed and validated in Postgres via a CHECK constraint (see the gorm tag).
type Interval string

const (
	Interval1s Interval = "1s"
	Interval5s Interval = "5s"
	Interval1m Interval = "1m"
	Interval5m Interval = "5m"
	Interval1h Interval = "1h"
	Interval1d Interval = "1d"
)

// IntervalForDuration maps a sampling duration to its stored interval label,
// reporting ok=false for any duration outside the predefined set. Callers use it
// to validate a configured interval and skip the job when it doesn't match,
// rather than silently coercing to a default label.
func IntervalForDuration(d time.Duration) (Interval, bool) {
	switch d {
	case time.Second:
		return Interval1s, true
	case 5 * time.Second:
		return Interval5s, true
	case time.Minute:
		return Interval1m, true
	case 5 * time.Minute:
		return Interval5m, true
	case time.Hour:
		return Interval1h, true
	case 24 * time.Hour:
		return Interval1d, true
	default:
		return "", false
	}
}

// Source identifies where an observation came from. The set is open (new
// exchanges get added over time), so it is a plain string in Postgres.
type Source string

const (
	SourceBinance Source = "binance"
	SourceKraken  Source = "kraken"
)

// LatestPrice is the newest observed value for a (base, quote) edge. One row
// per edge, always upserted. This is a fact table — the newest observation,
// never a derived pair.
type LatestPrice struct {
	Base      string    `gorm:"primaryKey;column:base" json:"base"`
	Quote     string    `gorm:"primaryKey;column:quote" json:"quote"`
	Price     float64   `json:"price"`
	Source    Source    `gorm:"type:text" json:"source"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (LatestPrice) TableName() string { return "latest_prices" }

// PriceCandle is an append-only OHLC observation in the native quote currency.
// Real observations only — never backfilled with fabricated data. The unique
// index makes klines backfill idempotent.
type PriceCandle struct {
	ID       uint64   `gorm:"primaryKey;autoIncrement" json:"id"`
	Base     string   `gorm:"uniqueIndex:uq_candle,priority:1;index:idx_candle_lookup,priority:1" json:"base"`
	Quote    string   `gorm:"uniqueIndex:uq_candle,priority:2;index:idx_candle_lookup,priority:2" json:"quote"`
	Interval Interval `gorm:"type:text;uniqueIndex:uq_candle,priority:3;index:idx_candle_lookup,priority:3;check:interval IN ('1s','5s','1m','5m','1h','1d')" json:"interval"`
	OpenTime time.Time `gorm:"uniqueIndex:uq_candle,priority:4;index:idx_candle_lookup,priority:4,sort:desc" json:"open_time"`
	Open     float64  `json:"open"`
	High     float64  `json:"high"`
	Low      float64  `json:"low"`
	Close    float64  `json:"close"`
	Volume   float64  `gorm:"default:0" json:"volume"`
	Source   Source   `gorm:"type:text;uniqueIndex:uq_candle,priority:5" json:"source"`
	// CreatedAt records when this row was written (append-only, never updated).
	// Distinct from OpenTime (market time); useful for tracing ingestion issues.
	CreatedAt time.Time `json:"created_at"`
}

func (PriceCandle) TableName() string { return "price_candles" }

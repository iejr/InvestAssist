// Package market holds READ-ONLY mirrors of the data market-feed owns:
// latest_prices (Redis HASH) and price_candles (Postgres). market-feed is the
// only writer; this service never migrates or writes them.
package market

import "time"

// Interval is the OHLC bucket label used in price_candles. History valuation
// uses the daily bucket (Interval1d): last close at-or-before a date.
type Interval string

const (
	Interval1s Interval = "1s"
	Interval5s Interval = "5s"
	Interval1m Interval = "1m"
	Interval5m Interval = "5m"
	Interval1h Interval = "1h"
	Interval1d Interval = "1d"
)

// LatestPrice is the newest observed value for one (base, quote) edge — the
// JSON value stored in Redis under HASH "latest_prices", field
// "{base}/{quote}". Base/Quote aren't part of the value; they come from
// splitting the field key (see pricing.snapshot).
type LatestPrice struct {
	Price     float64   `json:"price"`
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PriceCandle is one append-only OHLC observation in the native quote currency.
type PriceCandle struct {
	ID       uint64    `gorm:"column:id" json:"id"`
	Base     string    `gorm:"column:base" json:"base"`
	Quote    string    `gorm:"column:quote" json:"quote"`
	Interval Interval  `gorm:"column:interval" json:"interval"`
	OpenTime time.Time `gorm:"column:open_time" json:"open_time"`
	Open     float64   `json:"open"`
	High     float64   `json:"high"`
	Low      float64   `json:"low"`
	Close    float64   `json:"close"`
	Volume   float64   `json:"volume"`
	Source   string    `gorm:"type:text" json:"source"`
}

func (PriceCandle) TableName() string { return "price_candles" }

// Package model holds the strategy server's own tables. These are the only
// tables this service migrates and writes; market data (latest_prices,
// price_candles) is owned by market-feed and read read-only via internal/market.
package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StrategyType selects the contribution rule.
type StrategyType string

const (
	// StrategyTypeDCA contributes a fixed increment every interval.
	StrategyTypeDCA StrategyType = "DCA"
	// StrategyTypeVA tops the portfolio up to a growing target each interval.
	StrategyTypeVA StrategyType = "VA"
)

// Interval is the contribution cadence. Kept as a small closed set so date-step
// math (see internal/strategy) stays total.
type Interval string

const (
	IntervalWeekly  Interval = "weekly"
	IntervalMonthly Interval = "monthly"
)

// Strategy is a value-average / DCA plan.
//
// Two currency concepts live here (see DESIGN Decision D):
//   - Quote is the accounting currency, fixed for the strategy's lifetime. ALL
//     VA math (increment, target, actual, recommended contribution) is in Quote.
//   - Display currency is chosen per-view at read time and never stored here; it
//     is a presentation conversion only.
//
// Base + Quote are chosen at creation from what the feed actually carries
// (bases from latest_prices; quotes reachable from that base).
type Strategy struct {
	ID        uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	Name      string       `json:"name"`
	Type      StrategyType `json:"type"`
	Base      string       `json:"base"`  // asset accumulated, e.g. BTC
	Quote     string       `json:"quote"` // accounting currency, e.g. USDT
	StartDate time.Time    `json:"start_date"`
	Interval  Interval     `json:"interval"`
	Increment float64      `json:"increment"` // per-step target growth, in Quote units

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TransactionType is the direction of a user-recorded trade.
type TransactionType string

const (
	TransactionTypeBuy  TransactionType = "BUY"
	TransactionTypeSell TransactionType = "SELL"
)

// Transaction is a user-recorded execution. Shares and Price are exactly what
// the user entered (executed price); valuation prices come from the feed, not
// from here. Fees are recorded in their own currency + amount so cost-basis
// treatment can evolve later without a schema change.
type Transaction struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	StrategyID  uuid.UUID       `gorm:"type:uuid;index" json:"strategy_id"`
	Type        TransactionType `json:"type"`
	Shares      float64         `json:"shares"`
	Price       float64         `json:"price"`        // executed price in the strategy's quote
	FeeCurrency string          `json:"fee_currency"` // e.g. USDT, BNB; empty ⇒ no fee
	FeeAmount   float64         `json:"fee_amount"`
	Timestamp   time.Time       `json:"timestamp"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Strategy) BeforeCreate(*gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

func (t *Transaction) BeforeCreate(*gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

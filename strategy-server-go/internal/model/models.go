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

// Transaction is a user-recorded execution: what left the wallet (Spent) and
// what entered it (Gained). Amounts are net — fees, slippage, and anything
// else the exchange took are already folded in by the user, so there's no
// separate fee accounting here. Valuation prices still come from the feed,
// never from this record.
type Transaction struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	StrategyID uuid.UUID `gorm:"type:uuid;index" json:"strategy_id"`

	SpentSymbol  string  `json:"spent_symbol"`
	SpentAmount  float64 `json:"spent_amount"`
	GainedSymbol string  `json:"gained_symbol"`
	GainedAmount float64 `json:"gained_amount"`

	Memo      string    `json:"memo"` // free text; not used in any calculation
	Timestamp time.Time `json:"timestamp"`

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

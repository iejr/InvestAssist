// Package strategy holds the VA/DCA valuation math. Everything is computed in
// the strategy's QUOTE (accounting) currency, using money.Money (an exact
// decimal, not float64) for every value this service owns outright; a
// display currency, if different, is applied only as a presentation
// conversion (Decision D).
//
// Load returns a Strategy — a VA or DCA value that owns 100% of its own
// type-specific math (the contribution rule, and any auxiliary curve like
// VA's target) end to end. Each composes a portfolio (see portfolio.go) for
// everything universal — actual portfolio value, invested amount, date
// sampling, display conversion — which never branches on strategy type.
// There is no separate orchestration layer in between: a caller holding a
// Strategy calls GetHistory/GetSummary on it directly.
package strategy

import (
	"time"

	"strategy-server-go/internal/model"
	"strategy-server-go/internal/money"
	"strategy-server-go/internal/pricing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Strategy is the per-type entry point returned by Load: a VA or DCA
// strategy, ready to answer valuation queries for itself.
type Strategy interface {
	// GetHistory builds a value series in the requested display currency
	// (empty ⇒ the strategy's own quote). The date axis is independent of
	// the strategy's contribution interval — it's a bounded, evenly-spaced
	// sample (see sampleDates), not one point per interval step.
	//
	// rangeFrom, if non-zero, narrows the sampled window itself (not a
	// post-hoc filter) so maxPoints resolution is spent inside the
	// requested range rather than across the strategy's whole lifetime.
	// maxPoints bounds the number of points returned.
	GetHistory(display string, rangeFrom, now time.Time, maxPoints int) (*History, error)

	// GetSummary builds the headline panel: current value vs invested,
	// gain/loss, the next recommended contribution, and any strategy-
	// specific extras (e.g. VA's current target).
	GetSummary(display string, now time.Time) (*Summary, error)

	// DisplayCurrencies lists the currencies this strategy may be viewed
	// in: its own quote (always, zero conversion) plus everything
	// reachable from the quote.
	DisplayCurrencies() ([]string, string, error)
}

// SeriesMeta describes one named series found in HistoryPoint.Values, so a
// caller (the UI) can render every curve a strategy exposes without
// hardcoding which strategy types expose which keys. "actual" and
// "invested" are universal (every Strategy's History includes them); every
// further entry is added by that Strategy's own GetHistory.
type SeriesMeta struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// HistoryPoint is one sampled date's valuation. Values holds every curve at
// this point, keyed by SeriesMeta.Key, in the strategy's quote currency; a
// key's absence (rather than a null) means that value is unknown at this
// point (e.g. "actual" and "recommended_contribution" are absent when the
// price itself is missing). DisplayValues mirrors Values in the requested
// display currency, absent entirely when that conversion itself is
// unavailable at this date.
type HistoryPoint struct {
	Date       time.Time   `json:"date"`
	StepCount  int         `json:"step_count"`
	SharesHeld money.Money `json:"shares_held"`

	Values        map[string]money.Money `json:"values"`
	DisplayValues map[string]money.Money `json:"display_values,omitempty"`

	Missing bool `json:"missing"`
}

// History is the full response for the target-vs-actual chart.
type History struct {
	StrategyID uuid.UUID      `json:"strategy_id"`
	Base       string         `json:"base"`
	Quote      string         `json:"quote"`
	Display    string         `json:"display"`
	Series     []SeriesMeta   `json:"series"`
	Points     []HistoryPoint `json:"points"`
}

// Summary is the headline panel. Same Values/DisplayValues
// absence-means-unknown convention as HistoryPoint.
type Summary struct {
	StrategyID uuid.UUID `json:"strategy_id"`
	Base       string    `json:"base"`
	Quote      string    `json:"quote"`
	Display    string    `json:"display"`

	SharesHeld money.Money `json:"shares_held"`

	Values        map[string]money.Money `json:"values"`
	DisplayValues map[string]money.Money `json:"display_values,omitempty"`

	PriceStale bool `json:"price_stale"` // true when the live edge has no route/price
}

// Loader builds the Strategy for a given id. It holds only shared
// dependencies (db, pricer, the staleness override) — no per-strategy
// state — so one Loader serves every request.
type Loader struct {
	db                *gorm.DB
	pricer            *pricing.Pricer
	stalenessOverride time.Duration
}

// New builds a Loader. db reads the strategy tables; pricer valuates.
func New(db *gorm.DB, pricer *pricing.Pricer, stalenessOverride time.Duration) *Loader {
	return &Loader{db: db, pricer: pricer, stalenessOverride: stalenessOverride}
}

// Load reads a strategy row and its transactions, and returns the
// per-type Strategy that knows how to value them. strat.Type is validated
// at creation time (see internal/api), so an unknown type here is a
// DB-invariant violation, not a runtime condition callers need to handle.
func (l *Loader) Load(id uuid.UUID) (Strategy, error) {
	var strat model.Strategy
	if err := l.db.First(&strat, "id = ?", id).Error; err != nil {
		return nil, err
	}
	var txs []model.Transaction
	if err := l.db.Where("strategy_id = ?", id).Order("timestamp asc").Find(&txs).Error; err != nil {
		return nil, err
	}

	pf := portfolio{
		pricer:    l.pricer,
		base:      strat.Base,
		quote:     strat.Quote,
		start:     strat.StartDate,
		interval:  strat.Interval,
		increment: strat.Increment,
		txs:       txs,
		// Same value today; kept as two fields (see portfolio's
		// txStaleness doc) so they can diverge later without a signature
		// change.
		staleness:   maxStaleness(strat.Interval, l.stalenessOverride),
		txStaleness: maxStaleness(strat.Interval, l.stalenessOverride),
	}

	switch strat.Type {
	case model.StrategyTypeVA:
		return VA{pf: pf}, nil
	case model.StrategyTypeDCA:
		return DCA{pf: pf}, nil
	default:
		panic("strategy: unknown strategy type " + string(strat.Type))
	}
}

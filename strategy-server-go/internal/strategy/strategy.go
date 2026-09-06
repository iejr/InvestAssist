// Package strategy holds the VA/DCA valuation math. Everything is computed in
// the strategy's QUOTE (accounting) currency; a display currency, if different,
// is applied only as a presentation conversion (Decision D).
package strategy

import (
	"time"

	"strategy-server-go/internal/model"
	"strategy-server-go/internal/pricing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Service computes history and summaries for strategies.
type Service struct {
	db     *gorm.DB
	pricer *pricing.Pricer

	// stalenessOverride, when > 0, replaces the default per-strategy staleness
	// window (one interval-step).
	stalenessOverride time.Duration
}

// New builds a Service. db reads the strategy tables; pricer valuates.
func New(db *gorm.DB, pricer *pricing.Pricer, stalenessOverride time.Duration) *Service {
	return &Service{db: db, pricer: pricer, stalenessOverride: stalenessOverride}
}

// HistoryPoint is one interval date's valuation. Canonical numeric fields are in
// the strategy's quote currency; Display* mirrors are in the requested display
// currency (nil when the display conversion is missing at that date). Missing is
// true when the base/quote price itself is unavailable at the date.
type HistoryPoint struct {
	Date       time.Time `json:"date"`
	StepCount  int       `json:"step_count"`
	SharesHeld float64   `json:"shares_held"`

	Target      float64  `json:"target"`                  // quote units
	Actual      *float64 `json:"actual"`                  // quote units; nil ⇒ price missing
	Recommended *float64 `json:"recommended_contribution"` // quote units; nil ⇒ price missing (VA)

	DisplayTarget      *float64 `json:"display_target"`
	DisplayActual      *float64 `json:"display_actual"`
	DisplayRecommended *float64 `json:"display_recommended_contribution"`

	Missing bool `json:"missing"`
}

// History is the full response for the target-vs-actual chart.
type History struct {
	StrategyID uuid.UUID      `json:"strategy_id"`
	Base       string         `json:"base"`
	Quote      string         `json:"quote"`
	Display    string         `json:"display"`
	Points     []HistoryPoint `json:"points"`
}

// Summary is the headline panel: latest target vs actual plus the next
// recommended contribution. Current value uses live latest_prices.
type Summary struct {
	StrategyID uuid.UUID `json:"strategy_id"`
	Base       string    `json:"base"`
	Quote      string    `json:"quote"`
	Display    string    `json:"display"`

	SharesHeld    float64  `json:"shares_held"`
	Target        float64  `json:"target"`         // latest interval target, quote
	Actual        *float64 `json:"actual"`         // latest interval actual (candle), quote
	CurrentValue  *float64 `json:"current_value"`  // live value from latest_prices, quote
	TotalInvested float64  `json:"total_invested"` // quote
	GainLoss      *float64 `json:"gain_loss"`      // current_value - invested, quote
	NextRecommended *float64 `json:"next_recommended_contribution"` // quote

	DisplayCurrentValue    *float64 `json:"display_current_value"`
	DisplayTotalInvested   *float64 `json:"display_total_invested"`
	DisplayGainLoss        *float64 `json:"display_gain_loss"`
	DisplayNextRecommended *float64 `json:"display_next_recommended_contribution"`

	PriceStale bool `json:"price_stale"` // true when the live edge has no route/price
}

// GetHistory builds the interval-cadence series for a strategy in the requested
// display currency (empty ⇒ the strategy's own quote). rangeFrom, if non-zero,
// drops points before it (the series itself is always computed from start).
func (s *Service) GetHistory(id uuid.UUID, display string, rangeFrom time.Time, now time.Time) (*History, error) {
	strat, txs, err := s.load(id)
	if err != nil {
		return nil, err
	}
	if display == "" {
		display = strat.Quote
	}
	staleness := s.maxStaleness(strat)

	dates := generateDates(strat.StartDate, strat.Interval, now)
	points := make([]HistoryPoint, 0, len(dates))
	txIdx := 0
	var shares float64

	for i, d := range dates {
		// Fold in every transaction executed at-or-before this date.
		for txIdx < len(txs) && !txs[txIdx].Timestamp.After(d) {
			shares += signedShares(txs[txIdx])
			txIdx++
		}

		stepCount := i + 1
		target := float64(stepCount) * strat.Increment

		pt := HistoryPoint{
			Date:       d,
			StepCount:  stepCount,
			SharesHeld: shares,
			Target:     target,
		}

		price, ok, err := s.pricer.PriceAt(strat.Base, strat.Quote, d, staleness)
		if err != nil {
			return nil, err
		}
		if ok {
			actual := shares * price
			pt.Actual = &actual
			rec := recommended(strat, target, actual)
			pt.Recommended = &rec
		} else {
			pt.Missing = true
			// DCA's recommended contribution is fixed and doesn't need a price.
			if strat.Type == model.StrategyTypeDCA {
				rec := strat.Increment
				pt.Recommended = &rec
			}
		}

		s.applyDisplay(&pt, strat, display, d, staleness)

		if rangeFrom.IsZero() || !d.Before(rangeFrom) {
			points = append(points, pt)
		}
	}

	return &History{StrategyID: id, Base: strat.Base, Quote: strat.Quote, Display: display, Points: points}, nil
}

// GetSummary builds the headline panel using live latest_prices for the current
// value and the latest interval point for target/actual.
func (s *Service) GetSummary(id uuid.UUID, display string, now time.Time) (*Summary, error) {
	strat, txs, err := s.load(id)
	if err != nil {
		return nil, err
	}
	if display == "" {
		display = strat.Quote
	}

	dates := generateDates(strat.StartDate, strat.Interval, now)

	var sharesNow, invested float64
	for _, t := range txs {
		sharesNow += signedShares(t)
		invested += signedCost(t) + feeInQuote(t, strat.Quote)
	}

	sum := &Summary{
		StrategyID:    id,
		Base:          strat.Base,
		Quote:         strat.Quote,
		Display:       display,
		SharesHeld:    sharesNow,
		TotalInvested: invested,
	}

	stepCount := len(dates)
	if stepCount == 0 {
		stepCount = 1 // strategy starts in the future: first step is still ahead.
	}
	sum.Target = float64(stepCount) * strat.Increment

	// Live current value from latest_prices (not candles).
	price, _, ok, err := s.pricer.LatestPrice(strat.Base, strat.Quote)
	if err != nil {
		return nil, err
	}
	if ok {
		cv := sharesNow * price
		sum.CurrentValue = &cv
		sum.Actual = &cv // latest actual == live value
		gl := cv - invested
		sum.GainLoss = &gl

		// Next recommended contribution tops up to the NEXT interval target.
		nextTarget := float64(stepCount+1) * strat.Increment
		var rec float64
		if strat.Type == model.StrategyTypeVA {
			rec = nextTarget - cv
		} else {
			rec = strat.Increment
		}
		sum.NextRecommended = &rec
	} else {
		sum.PriceStale = true
		if strat.Type == model.StrategyTypeDCA {
			rec := strat.Increment
			sum.NextRecommended = &rec
		}
	}

	s.applySummaryDisplay(sum, strat, display)
	return sum, nil
}

// DisplayCurrencies lists the currencies a strategy may be viewed in: its own
// quote (always, zero conversion) plus everything reachable from the quote.
func (s *Service) DisplayCurrencies(id uuid.UUID) (options []string, def string, err error) {
	var strat model.Strategy
	if err := s.db.First(&strat, "id = ?", id).Error; err != nil {
		return nil, "", err
	}
	reachable, err := s.pricer.ReachableQuotes(strat.Quote)
	if err != nil {
		return nil, "", err
	}
	options = append([]string{strat.Quote}, reachable...)
	return options, strat.Quote, nil
}

// --- internals ---

func (s *Service) load(id uuid.UUID) (model.Strategy, []model.Transaction, error) {
	var strat model.Strategy
	if err := s.db.First(&strat, "id = ?", id).Error; err != nil {
		return strat, nil, err
	}
	var txs []model.Transaction
	if err := s.db.Where("strategy_id = ?", id).Order("timestamp asc").Find(&txs).Error; err != nil {
		return strat, nil, err
	}
	return strat, txs, nil
}

// applyDisplay converts a point's quote values into the display currency using
// the as-of quote→display rate at the point's date. If display == quote the
// rate is 1; if the conversion edge is missing/stale the display fields stay nil.
func (s *Service) applyDisplay(pt *HistoryPoint, strat model.Strategy, display string, d time.Time, staleness time.Duration) {
	factor, ok := s.displayFactorAt(strat.Quote, display, d, staleness)
	if !ok {
		return
	}
	t := pt.Target * factor
	pt.DisplayTarget = &t
	if pt.Actual != nil {
		v := *pt.Actual * factor
		pt.DisplayActual = &v
	}
	if pt.Recommended != nil {
		v := *pt.Recommended * factor
		pt.DisplayRecommended = &v
	}
}

func (s *Service) applySummaryDisplay(sum *Summary, strat model.Strategy, display string) {
	factor, ok := s.displayFactorLatest(strat.Quote, display)
	if !ok {
		return
	}
	if sum.CurrentValue != nil {
		v := *sum.CurrentValue * factor
		sum.DisplayCurrentValue = &v
	}
	inv := sum.TotalInvested * factor
	sum.DisplayTotalInvested = &inv
	if sum.GainLoss != nil {
		v := *sum.GainLoss * factor
		sum.DisplayGainLoss = &v
	}
	if sum.NextRecommended != nil {
		v := *sum.NextRecommended * factor
		sum.DisplayNextRecommended = &v
	}
}

func (s *Service) displayFactorAt(quote, display string, d time.Time, staleness time.Duration) (float64, bool) {
	if display == quote {
		return 1, true
	}
	f, ok, err := s.pricer.PriceAt(quote, display, d, staleness)
	if err != nil || !ok {
		return 0, false
	}
	return f, true
}

func (s *Service) displayFactorLatest(quote, display string) (float64, bool) {
	if display == quote {
		return 1, true
	}
	f, _, ok, err := s.pricer.LatestPrice(quote, display)
	if err != nil || !ok {
		return 0, false
	}
	return f, true
}

// maxStaleness is one interval-step by default (weekly ⇒ ~1 week, monthly ⇒
// ~1 month), overridable via config.
func (s *Service) maxStaleness(strat model.Strategy) time.Duration {
	if s.stalenessOverride > 0 {
		return s.stalenessOverride
	}
	if strat.Interval == model.IntervalWeekly {
		return 7 * 24 * time.Hour
	}
	return 31 * 24 * time.Hour
}

// recommended is the VA/DCA contribution in quote units for a computed point.
func recommended(strat model.Strategy, target, actual float64) float64 {
	if strat.Type == model.StrategyTypeVA {
		return target - actual // negative ⇒ sell
	}
	return strat.Increment
}

func signedShares(t model.Transaction) float64 {
	if t.Type == model.TransactionTypeSell {
		return -t.Shares
	}
	return t.Shares
}

// signedCost is the quote cash moved by a trade (naive: sells reduce basis by
// their proceeds — a noted v1 limitation).
func signedCost(t model.Transaction) float64 {
	c := t.Shares * t.Price
	if t.Type == model.TransactionTypeSell {
		return -c
	}
	return c
}

// feeInQuote folds a fee into invested cash only when it is denominated in the
// strategy's quote; other fee currencies are ignored in v1 (noted limitation).
func feeInQuote(t model.Transaction, quote string) float64 {
	if t.FeeCurrency == quote {
		return t.FeeAmount
	}
	return 0
}

// generateDates returns interval dates from start up to (and including) now.
func generateDates(start time.Time, interval model.Interval, now time.Time) []time.Time {
	var out []time.Time
	cur := start
	for !cur.After(now) {
		out = append(out, cur)
		if interval == model.IntervalWeekly {
			cur = cur.AddDate(0, 0, 7)
		} else {
			cur = cur.AddDate(0, 1, 0)
		}
	}
	return out
}

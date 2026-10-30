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

	Target      float64  `json:"target"`                   // quote units
	Actual      *float64 `json:"actual"`                   // quote units; nil ⇒ price missing
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

	SharesHeld      float64  `json:"shares_held"`
	Target          float64  `json:"target"`                        // latest interval target, quote
	Actual          *float64 `json:"actual"`                        // latest interval actual (candle), quote
	CurrentValue    *float64 `json:"current_value"`                 // live value from latest_prices, quote
	TotalInvested   float64  `json:"total_invested"`                // quote
	GainLoss        *float64 `json:"gain_loss"`                     // current_value - invested, quote
	NextRecommended *float64 `json:"next_recommended_contribution"` // quote

	DisplayCurrentValue    *float64 `json:"display_current_value"`
	DisplayTotalInvested   *float64 `json:"display_total_invested"`
	DisplayGainLoss        *float64 `json:"display_gain_loss"`
	DisplayNextRecommended *float64 `json:"display_next_recommended_contribution"`

	PriceStale bool `json:"price_stale"` // true when the live edge has no route/price
}

// GetHistory builds a value series for a strategy in the requested display
// currency (empty ⇒ the strategy's own quote). The date axis is independent
// of the strategy's contribution interval — it's a bounded, evenly-spaced
// sample (see sampleDates), not one point per interval step. Target is
// computed via ExpectedTarget, which assumes linear growth between the
// strategy's own weekly/monthly interval boundaries, so the chart doesn't
// show a staircase just because its sample dates don't line up with the
// contribution cadence. StepCount is still the raw discrete elapsed-step
// count (DB-free, cheap) for callers that want the un-smoothed cadence.
//
// rangeFrom, if non-zero, narrows the sampled window itself (not a post-hoc
// filter) so maxPoints resolution is spent inside the requested range rather
// than across the strategy's whole lifetime. maxPoints bounds the number of
// points returned; pricing is resolved (route discovery) once per request,
// not once per date, regardless of how many points are sampled.
func (s *Service) GetHistory(id uuid.UUID, display string, rangeFrom time.Time, now time.Time, maxPoints int) (*History, error) {
	strat, txs, err := s.load(id)
	if err != nil {
		return nil, err
	}
	if display == "" {
		display = strat.Quote
	}
	staleness := s.maxStaleness(strat)

	// Pure date math (no DB/pricing calls) — anchors Target/Recommended to
	// the strategy's own cadence independent of the chart's sample dates.
	intervalDates := generateDates(strat.StartDate, strat.Interval, now)

	effectiveStart := strat.StartDate
	if !rangeFrom.IsZero() && rangeFrom.After(effectiveStart) {
		effectiveStart = rangeFrom
	}
	dates := sampleDates(effectiveStart, now, maxPoints)

	baseRoute, baseOK, err := s.pricer.ResolveRoute(strat.Base, strat.Quote)
	if err != nil {
		return nil, err
	}
	displaySame := display == strat.Quote
	var displayRoute *pricing.Route
	if !displaySame {
		displayRoute, _, err = s.pricer.ResolveRoute(strat.Quote, display)
		if err != nil {
			return nil, err
		}
	}

	points := make([]HistoryPoint, 0, len(dates))
	txIdx := 0
	intervalIdx := 0
	var shares float64

	for _, d := range dates {
		// Fold in every transaction executed at-or-before this date.
		for txIdx < len(txs) && !txs[txIdx].Timestamp.After(d) {
			shares += signedShares(txs[txIdx], strat)
			txIdx++
		}
		// Elapsed interval steps as-of this date — independent of whether d
		// itself lands on an interval boundary.
		for intervalIdx < len(intervalDates) && !intervalDates[intervalIdx].After(d) {
			intervalIdx++
		}
		stepCount := intervalIdx
		target := ExpectedTarget(strat.StartDate, strat.Interval, strat.Increment, d)

		pt := HistoryPoint{
			Date:       d,
			StepCount:  stepCount,
			SharesHeld: shares,
			Target:     target,
		}

		var price float64
		var priceOK bool
		if baseOK {
			price, priceOK, err = s.pricer.PriceAtRoute(baseRoute, d, staleness)
			if err != nil {
				return nil, err
			}
		}
		if priceOK {
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

		s.applyDisplay(&pt, displaySame, displayRoute, d, staleness)

		points = append(points, pt)
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
		sharesNow += signedShares(t, strat)
		invested += signedCost(t, strat)
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
// the as-of quote→display rate at the point's date, via an already-resolved
// Route (displaySame true ⇒ rate is 1, no lookup). If the conversion is
// missing/stale the display fields stay nil.
func (s *Service) applyDisplay(pt *HistoryPoint, displaySame bool, displayRoute *pricing.Route, d time.Time, staleness time.Duration) {
	var factor float64
	if displaySame {
		factor = 1
	} else {
		if displayRoute == nil {
			return
		}
		f, ok, err := s.pricer.PriceAtRoute(displayRoute, d, staleness)
		if err != nil || !ok {
			return
		}
		factor = f
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

// signedShares is the strategy's base asset moved by a transaction: positive
// when base was gained (bought), negative when base was spent (sold). A
// transaction touching neither side of strat.Base contributes nothing.
func signedShares(t model.Transaction, strat model.Strategy) float64 {
	switch {
	case t.GainedSymbol == strat.Base:
		return t.GainedAmount
	case t.SpentSymbol == strat.Base:
		return -t.SpentAmount
	default:
		return 0
	}
}

// signedCost is the quote cash moved by a transaction (naive: proceeds from
// a sale reduce basis directly — a noted v1 limitation). A transaction
// touching neither side of strat.Quote contributes nothing.
func signedCost(t model.Transaction, strat model.Strategy) float64 {
	switch {
	case t.SpentSymbol == strat.Quote:
		return t.SpentAmount
	case t.GainedSymbol == strat.Quote:
		return -t.GainedAmount
	default:
		return 0
	}
}

// ExpectedTarget returns the VA/DCA target portfolio value at an arbitrary
// date d, assuming the target grows linearly between interval boundaries
// instead of jumping by a full increment exactly at each one. It agrees with
// the discrete step-count target (stepCount*increment) at every boundary
// date — start, start+interval, start+2*interval, ... — and interpolates a
// straight line in between, so a chart sampled at dates unrelated to the
// strategy's own cadence (see sampleDates) doesn't show a staircase. Dates
// before start return 0.
func ExpectedTarget(start time.Time, interval model.Interval, increment float64, d time.Time) float64 {
	if d.Before(start) {
		return 0
	}
	step, prev, next := intervalBounds(start, interval, d)
	var frac float64
	if span := next.Sub(prev); span > 0 {
		frac = d.Sub(prev).Seconds() / span.Seconds()
	}
	return (float64(step) + 1 + frac) * increment
}

// intervalBounds returns the number of whole interval steps elapsed strictly
// before d (step), and the interval boundary immediately at-or-before d
// (prev) and the next one after it (next). Assumes d is at-or-after start.
func intervalBounds(start time.Time, interval model.Interval, d time.Time) (step int, prev, next time.Time) {
	if interval == model.IntervalWeekly {
		week := 7 * 24 * time.Hour
		step = int(d.Sub(start) / week)
		prev = start.Add(time.Duration(step) * week)
		next = prev.Add(week)
		return
	}
	// Monthly: calendar months vary in length, so estimate the elapsed
	// month count from the calendar difference, then correct by a step or
	// two — AddDate normalizes day overflow (e.g. Jan 31 + 1 month), which
	// can throw the estimate off by one.
	step = (d.Year()-start.Year())*12 + int(d.Month()) - int(start.Month())
	for start.AddDate(0, step, 0).After(d) {
		step--
	}
	for !start.AddDate(0, step+1, 0).After(d) {
		step++
	}
	prev = start.AddDate(0, step, 0)
	next = start.AddDate(0, step+1, 0)
	return
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

// sampleDates returns the chart's date axis for [start, end], independent of
// any strategy interval. When the span fits within maxPoints at daily
// resolution, every day is returned. Otherwise points are evenly spaced by
// day-count, with the first and last points pinned to start and end so the
// series never looks truncated or stops short of "today". Both bounds are
// normalized to UTC midnight so the result doesn't depend on the server
// process's local timezone. Returns nil if start is after end (e.g. a
// strategy that hasn't started yet).
func sampleDates(start, end time.Time, maxPoints int) []time.Time {
	if maxPoints < 2 {
		maxPoints = 2
	}
	start = start.UTC().Truncate(24 * time.Hour)
	end = end.UTC().Truncate(24 * time.Hour)
	if start.After(end) {
		return nil
	}

	totalDays := int(end.Sub(start) / (24 * time.Hour))
	if totalDays+1 <= maxPoints {
		out := make([]time.Time, 0, totalDays+1)
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			out = append(out, d)
		}
		return out
	}

	step := (totalDays + maxPoints - 2) / (maxPoints - 1) // ceil(totalDays/(maxPoints-1))
	out := make([]time.Time, 0, maxPoints)
	for d := start; d.Before(end); d = d.AddDate(0, 0, step) {
		out = append(out, d)
	}
	return append(out, end)
}

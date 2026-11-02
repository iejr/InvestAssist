// Package strategy holds the VA/DCA valuation math. Everything is computed in
// the strategy's QUOTE (accounting) currency, using money.Money (an exact
// decimal, not float64) for every value this service owns outright; a
// display currency, if different, is applied only as a presentation
// conversion (Decision D). Strategy-type-specific behavior (the recommended-
// contribution rule, and any auxiliary curves like VA's target) is isolated
// behind the Plan interface (see plan.go) — everything else here (actual
// portfolio value, invested amount, date sampling) is universal and never
// branches on Strategy.Type.
package strategy

import (
	"sort"
	"time"

	"strategy-server-go/internal/model"
	"strategy-server-go/internal/money"
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

// SeriesMeta describes one named series found in HistoryPoint.Values, so a
// caller (the UI) can render every curve a strategy exposes without
// hardcoding which strategy types expose which keys. "actual", "invested"
// and "recommended_contribution" are always present (universal or
// type-determined-but-always-answered); any further entries come from the
// strategy's own Plan.ExtraLabels (e.g. VA's "target").
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

// Summary is the headline panel: current value vs invested, gain/loss, plus
// the next recommended contribution and any strategy-specific extras (e.g.
// VA's latest target). Same Values/DisplayValues absence-means-unknown
// convention as HistoryPoint.
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

// GetHistory builds a value series for a strategy in the requested display
// currency (empty ⇒ the strategy's own quote). The date axis is independent
// of the strategy's contribution interval — it's a bounded, evenly-spaced
// sample (see sampleDates), not one point per interval step. Any VA/DCA
// target curve comes from the strategy's own Plan (see plan.go), which
// assumes linear growth between the strategy's own weekly/monthly interval
// boundaries, so the chart doesn't show a staircase just because its sample
// dates don't line up with the contribution cadence. StepCount is still the
// raw discrete elapsed-step count (DB-free, cheap) for callers that want the
// un-smoothed cadence.
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
	plan := NewPlan(strat)

	// Pure date math (no DB/pricing calls) — StepCount's cadence, independent
	// of the chart's sample dates.
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
	shares := money.Zero
	invested := money.Zero

	for _, d := range dates {
		// Fold in every transaction executed at-or-before this date.
		for txIdx < len(txs) && !txs[txIdx].Timestamp.After(d) {
			shares = shares.Add(signedShares(txs[txIdx], strat))
			invested = invested.Add(signedCost(txs[txIdx], strat))
			txIdx++
		}
		// Elapsed interval steps as-of this date — independent of whether d
		// itself lands on an interval boundary.
		for intervalIdx < len(intervalDates) && !intervalDates[intervalIdx].After(d) {
			intervalIdx++
		}

		pt := HistoryPoint{
			Date:       d,
			StepCount:  intervalIdx,
			SharesHeld: shares,
			Values:     map[string]money.Money{"invested": invested},
		}

		var price float64
		var priceOK bool
		if baseOK {
			price, priceOK, err = s.pricer.PriceAtRoute(baseRoute, d, staleness)
			if err != nil {
				return nil, err
			}
		}
		var actual *money.Money
		if priceOK {
			a := money.FromPrice(price).Mul(shares)
			actual = &a
			pt.Values["actual"] = a
		} else {
			pt.Missing = true
		}
		if rec, ok := plan.Recommended(d, actual); ok {
			pt.Values["recommended_contribution"] = rec
		}
		for k, v := range plan.Extras(d) {
			pt.Values[k] = v
		}

		s.applyDisplay(&pt, displaySame, displayRoute, d, staleness)

		points = append(points, pt)
	}

	return &History{
		StrategyID: id,
		Base:       strat.Base,
		Quote:      strat.Quote,
		Display:    display,
		Series:     buildSeries(plan.ExtraLabels()),
		Points:     points,
	}, nil
}

// GetSummary builds the headline panel using live latest_prices for the
// current value and the strategy's own Plan for the next recommended
// contribution and any extras.
func (s *Service) GetSummary(id uuid.UUID, display string, now time.Time) (*Summary, error) {
	strat, txs, err := s.load(id)
	if err != nil {
		return nil, err
	}
	if display == "" {
		display = strat.Quote
	}
	plan := NewPlan(strat)

	sharesNow := money.Zero
	invested := money.Zero
	for _, t := range txs {
		sharesNow = sharesNow.Add(signedShares(t, strat))
		invested = invested.Add(signedCost(t, strat))
	}

	sum := &Summary{
		StrategyID: id,
		Base:       strat.Base,
		Quote:      strat.Quote,
		Display:    display,
		SharesHeld: sharesNow,
		Values:     map[string]money.Money{"total_invested": invested},
	}
	for k, v := range plan.Extras(now) {
		sum.Values[k] = v
	}

	// Live current value from latest_prices (not candles).
	price, _, ok, err := s.pricer.LatestPrice(strat.Base, strat.Quote)
	if err != nil {
		return nil, err
	}
	if ok {
		cv := money.FromPrice(price).Mul(sharesNow)
		sum.Values["current_value"] = cv
		sum.Values["gain_loss"] = cv.Sub(invested)
		if rec, recOK := plan.NextRecommended(now, &cv); recOK {
			sum.Values["next_recommended_contribution"] = rec
		}
	} else {
		sum.PriceStale = true
		if rec, recOK := plan.NextRecommended(now, nil); recOK {
			sum.Values["next_recommended_contribution"] = rec
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

// buildSeries is History's series manifest: the universal/always-answered
// keys, followed by the strategy's own extras (if any), key-sorted for a
// stable, deterministic order.
func buildSeries(extra map[string]string) []SeriesMeta {
	series := []SeriesMeta{
		{Key: "actual", Label: "Actual"},
		{Key: "invested", Label: "Invested"},
		{Key: "recommended_contribution", Label: "Recommended Contribution"},
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		series = append(series, SeriesMeta{Key: k, Label: extra[k]})
	}
	return series
}

// applyDisplay converts a point's quote-currency Values into the display
// currency using the as-of quote→display rate at the point's date, via an
// already-resolved Route (displaySame true ⇒ rate is 1, no lookup). If the
// conversion is missing/stale, DisplayValues stays nil (absent in JSON).
func (s *Service) applyDisplay(pt *HistoryPoint, displaySame bool, displayRoute *pricing.Route, d time.Time, staleness time.Duration) {
	var factor money.Money
	if displaySame {
		factor = money.FromInt(1)
	} else {
		if displayRoute == nil {
			return
		}
		f, ok, err := s.pricer.PriceAtRoute(displayRoute, d, staleness)
		if err != nil || !ok {
			return
		}
		factor = money.FromPrice(f)
	}
	dv := make(map[string]money.Money, len(pt.Values))
	for k, v := range pt.Values {
		dv[k] = v.Mul(factor)
	}
	pt.DisplayValues = dv
}

func (s *Service) applySummaryDisplay(sum *Summary, strat model.Strategy, display string) {
	factor, ok := s.displayFactorLatest(strat.Quote, display)
	if !ok {
		return
	}
	dv := make(map[string]money.Money, len(sum.Values))
	for k, v := range sum.Values {
		dv[k] = v.Mul(factor)
	}
	sum.DisplayValues = dv
}

func (s *Service) displayFactorLatest(quote, display string) (money.Money, bool) {
	if display == quote {
		return money.FromInt(1), true
	}
	f, _, ok, err := s.pricer.LatestPrice(quote, display)
	if err != nil || !ok {
		return money.Zero, false
	}
	return money.FromPrice(f), true
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

// signedShares is the strategy's base asset moved by a transaction: positive
// when base was gained (bought), negative when base was spent (sold). A
// transaction touching neither side of strat.Base contributes nothing.
func signedShares(t model.Transaction, strat model.Strategy) money.Money {
	switch {
	case t.GainedSymbol == strat.Base:
		return t.GainedAmount
	case t.SpentSymbol == strat.Base:
		return t.SpentAmount.Neg()
	default:
		return money.Zero
	}
}

// signedCost is the quote cash moved by a transaction (naive: proceeds from
// a sale reduce basis directly — a noted v1 limitation). A transaction
// touching neither side of strat.Quote contributes nothing.
func signedCost(t model.Transaction, strat model.Strategy) money.Money {
	switch {
	case t.SpentSymbol == strat.Quote:
		return t.SpentAmount
	case t.GainedSymbol == strat.Quote:
		return t.GainedAmount.Neg()
	default:
		return money.Zero
	}
}

// ExpectedTarget returns the VA/DCA target portfolio value at an arbitrary
// date d, modeled as a straight line expected = grad*(d-start) + b: grad is
// increment/interval-length and b is the value already due at the
// strategy's own start date. b is one full increment — the user is assumed
// to invest immediately at start rather than wait a full cycle — so the
// line reads increment at start, 2*increment at start+1*interval, and so
// on, agreeing with the discrete step-count target (stepCount*increment) at
// every boundary while interpolating linearly in between. Every date in
// this system is day-aligned (see sampleDates), so growth is computed in
// whole days — increment is multiplied by the elapsed day count before
// dividing by the period's day length, rather than pre-dividing into a
// per-day rate, to avoid losing precision in that intermediate ratio. Dates
// before start return 0.
func ExpectedTarget(start time.Time, interval model.Interval, increment money.Money, d time.Time) money.Money {
	if d.Before(start) {
		return money.Zero
	}
	step, prev, next := intervalBounds(start, interval, d)
	totalDays := daysBetween(prev, next)
	elapsedDays := daysBetween(prev, d)
	whole := increment.Mul(money.FromInt(step + 1)) // step + b, b = 1
	if totalDays <= 0 {
		return whole
	}
	frac := increment.Mul(money.FromInt(elapsedDays)).Div(money.FromInt(totalDays))
	return whole.Add(frac)
}

// daysBetween returns the whole number of days from a to b.
func daysBetween(a, b time.Time) int {
	return int(b.Sub(a) / (24 * time.Hour))
}

// intervalBounds returns the number of whole interval steps elapsed strictly
// before d (step), and the interval boundary immediately at-or-before d
// (prev) and the next one after it (next). Assumes d is at-or-after start.
func intervalBounds(start time.Time, interval model.Interval, d time.Time) (step int, prev, next time.Time) {
	if interval == model.IntervalWeekly {
		step = daysBetween(start, d) / 7
		prev = start.AddDate(0, 0, step*7)
		next = prev.AddDate(0, 0, 7)
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

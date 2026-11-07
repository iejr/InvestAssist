package strategy

import (
	"time"

	"strategy-server-go/internal/model"
	"strategy-server-go/internal/money"
	"strategy-server-go/internal/pricing"
)

// portfolio is the valuation engine shared by every strategy type: folding
// transactions into shares/invested, pricing the actual portfolio value,
// sampling the chart's date axis, and converting to a display currency.
// None of this varies by strategy type, so it lives here once rather than
// in each type; VA and DCA (see va.go, dca.go) each compose a portfolio by
// value and layer only their own contribution-rule math on top of what it
// returns.
type portfolio struct {
	pricer *pricing.Pricer

	base, quote string
	start       time.Time
	interval    model.Interval
	increment   money.Money // per-step contribution; storage is shared, meaning is type-specific

	txs []model.Transaction

	staleness time.Duration
}

// dates is GetHistory's chart date axis: a bounded, evenly-spaced sample
// between the later of the strategy's start / the requested range, and now.
// Independent of the strategy's own contribution interval (see sampleDates).
func (p portfolio) dates(rangeFrom, now time.Time, maxPoints int) []time.Time {
	effectiveStart := p.start
	if !rangeFrom.IsZero() && rangeFrom.After(effectiveStart) {
		effectiveStart = rangeFrom
	}
	return sampleDates(effectiveStart, now, maxPoints)
}

// history computes the universal per-date curves — invested and actual, in
// quote currency — plus StepCount/Missing, for the given dates. It returns
// raw quote-currency values with no Display/DisplayValues set: the caller
// (VA/DCA's GetHistory) merges in its own type-specific curves first, then
// calls toDisplay once over the merged result, so a type's extra curves get
// converted too instead of being added after conversion already ran.
func (p portfolio) history(dates []time.Time) (*History, error) {
	baseRoute, baseOK, err := p.pricer.ResolveRoute(p.base, p.quote)
	if err != nil {
		return nil, err
	}

	points := make([]HistoryPoint, 0, len(dates))
	txIdx := 0
	shares := money.Zero
	invested := money.Zero

	for _, d := range dates {
		// Fold in every transaction executed at-or-before this date.
		for txIdx < len(p.txs) && !p.txs[txIdx].Timestamp.After(d) {
			shares = shares.Add(p.signedShares(p.txs[txIdx]))
			invested = invested.Add(p.signedCost(p.txs[txIdx]))
			txIdx++
		}

		pt := HistoryPoint{
			Date:       d,
			StepCount:  stepCountAt(p.start, p.interval, d),
			SharesHeld: shares,
			Values:     map[string]money.Money{"invested": invested},
		}

		var priceOK bool
		var price float64
		if baseOK {
			price, priceOK, err = p.pricer.PriceAtRoute(baseRoute, d, p.staleness)
			if err != nil {
				return nil, err
			}
		}
		if priceOK {
			pt.Values["actual"] = money.FromPrice(price).Mul(shares)
		} else {
			pt.Missing = true
		}

		points = append(points, pt)
	}

	return &History{
		Base:   p.base,
		Quote:  p.quote,
		Series: []SeriesMeta{{Key: "actual", Label: "Actual"}, {Key: "invested", Label: "Invested"}},
		Points: points,
	}, nil
}

// toDisplay sets h.Display (defaulting "" to the strategy's own quote) and
// converts every point's already-merged Values into DisplayValues, via the
// quote→display rate at each point's date. Route resolution happens once
// per call, not once per point. If the route itself doesn't exist,
// DisplayValues stays absent on every point; a per-date rate that's merely
// stale/missing leaves just that point's DisplayValues absent.
func (p portfolio) toDisplay(h *History, display string) error {
	if display == "" {
		display = p.quote
	}
	h.Display = display

	if display == p.quote {
		for i := range h.Points {
			h.Points[i].DisplayValues = cloneValues(h.Points[i].Values)
		}
		return nil
	}

	route, ok, err := p.pricer.ResolveRoute(p.quote, display)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	for i := range h.Points {
		pt := &h.Points[i]
		f, ok, err := p.pricer.PriceAtRoute(route, pt.Date, p.staleness)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		pt.DisplayValues = scaleValues(pt.Values, money.FromPrice(f))
	}
	return nil
}

// summary computes the universal totals — total invested (always), current
// value/gain-loss (only when a live price exists) — in quote currency.
// PriceStale is true when no live price exists for the strategy's pair. The
// caller (VA/DCA's GetSummary) adds its own next-recommended-contribution
// (and any extras) before calling toDisplaySummary.
func (p portfolio) summary(now time.Time) (*Summary, error) {
	sharesNow := money.Zero
	invested := money.Zero
	for _, t := range p.txs {
		sharesNow = sharesNow.Add(p.signedShares(t))
		invested = invested.Add(p.signedCost(t))
	}

	sum := &Summary{
		Base:       p.base,
		Quote:      p.quote,
		SharesHeld: sharesNow,
		Values:     map[string]money.Money{"total_invested": invested},
	}

	// Live current value from latest_prices (not candles).
	price, _, ok, err := p.pricer.LatestPrice(p.base, p.quote)
	if err != nil {
		return nil, err
	}
	if !ok {
		sum.PriceStale = true
		return sum, nil
	}
	cv := money.FromPrice(price).Mul(sharesNow)
	sum.Values["current_value"] = cv
	sum.Values["gain_loss"] = cv.Sub(invested)
	return sum, nil
}

// toDisplaySummary is toDisplay's Summary counterpart, over the single
// latest rate rather than a per-date series.
func (p portfolio) toDisplaySummary(sum *Summary, display string) error {
	if display == "" {
		display = p.quote
	}
	sum.Display = display

	if display == p.quote {
		sum.DisplayValues = cloneValues(sum.Values)
		return nil
	}
	f, _, ok, err := p.pricer.LatestPrice(p.quote, display)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	sum.DisplayValues = scaleValues(sum.Values, money.FromPrice(f))
	return nil
}

// displayCurrencies lists the currencies this strategy may be viewed in:
// its own quote (always, zero conversion) plus everything reachable from
// the quote.
func (p portfolio) displayCurrencies() (options []string, def string, err error) {
	reachable, err := p.pricer.ReachableQuotes(p.quote)
	if err != nil {
		return nil, "", err
	}
	return append([]string{p.quote}, reachable...), p.quote, nil
}

// signedShares is the strategy's base asset moved by a transaction: positive
// when base was gained (bought), negative when base was spent (sold). A
// transaction touching neither side of p.base contributes nothing.
func (p portfolio) signedShares(t model.Transaction) money.Money {
	switch {
	case t.GainedSymbol == p.base:
		return t.GainedAmount
	case t.SpentSymbol == p.base:
		return t.SpentAmount.Neg()
	default:
		return money.Zero
	}
}

// signedCost is the quote cash moved by a transaction (naive: proceeds from
// a sale reduce basis directly — a noted v1 limitation). A transaction
// touching neither side of p.quote contributes nothing.
func (p portfolio) signedCost(t model.Transaction) money.Money {
	switch {
	case t.SpentSymbol == p.quote:
		return t.SpentAmount
	case t.GainedSymbol == p.quote:
		return t.GainedAmount.Neg()
	default:
		return money.Zero
	}
}

func cloneValues(v map[string]money.Money) map[string]money.Money {
	return scaleValues(v, money.FromInt(1))
}

func scaleValues(v map[string]money.Money, factor money.Money) map[string]money.Money {
	out := make(map[string]money.Money, len(v))
	for k, val := range v {
		out[k] = val.Mul(factor)
	}
	return out
}

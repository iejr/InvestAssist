package strategy

import (
	"log"
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
//
// base is matched exactly (see signedShares): it names one specific asset,
// so a different asset is never folded into its share count no matter how
// it's denominated. quote is NOT matched exactly for cost purposes (see
// costConverter): a transaction's cash leg is still cash even when it's in
// a different, but convertible, currency (e.g. paying in USDC for a
// USDT-quoted strategy) — that's converted via the pricer, not ignored.
type portfolio struct {
	pricer *pricing.Pricer

	base, quote string
	start       time.Time
	interval    model.Interval
	increment   money.Money // per-step contribution; storage is shared, meaning is type-specific

	txs []model.Transaction

	staleness time.Duration // max price age for valuing the strategy's own base/quote and display conversions
	// txStaleness bounds how old a price may be when converting a
	// transaction's non-quote cash leg into quote (see costConverter). A
	// separate field from staleness — same value today, computed the same
	// way — so the two can be tuned independently later without this
	// becoming a breaking change.
	txStaleness time.Duration
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
	costs := p.newCostConverter()

	points := make([]HistoryPoint, 0, len(dates))
	txIdx := 0
	shares := money.Zero
	invested := money.Zero
	investedKnown := true

	for _, d := range dates {
		// Fold in every transaction executed at-or-before this date. Once
		// any transaction's cost can't be converted, invested is a
		// permanently tainted running sum — it stays unknown (key absent)
		// for this and every later point in this response, rather than
		// quietly under-reporting it.
		for txIdx < len(p.txs) && !p.txs[txIdx].Timestamp.After(d) {
			shares = shares.Add(p.signedShares(p.txs[txIdx]))
			if cost, ok := costs.cost(p.txs[txIdx]); ok {
				invested = invested.Add(cost)
			} else {
				investedKnown = false
			}
			txIdx++
		}

		pt := HistoryPoint{
			Date:       d,
			StepCount:  stepCountAt(p.start, p.interval, d),
			SharesHeld: shares,
			Values:     map[string]money.Money{},
		}
		if investedKnown {
			pt.Values["invested"] = invested
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
	investedKnown := true
	costs := p.newCostConverter()
	for _, t := range p.txs {
		sharesNow = sharesNow.Add(p.signedShares(t))
		if cost, ok := costs.cost(t); ok {
			invested = invested.Add(cost)
		} else {
			investedKnown = false
		}
	}

	sum := &Summary{
		Base:       p.base,
		Quote:      p.quote,
		SharesHeld: sharesNow,
		Values:     map[string]money.Money{},
	}
	if investedKnown {
		sum.Values["total_invested"] = invested
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
	// gain_loss needs invested too; stays absent if that's unknown, same
	// as total_invested.
	if investedKnown {
		sum.Values["gain_loss"] = cv.Sub(invested)
	}
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

// costConverter is the quote cash moved by each transaction it's asked
// about, converting a cash leg denominated in something other than the
// strategy's own quote (e.g. paying in USDC for a USDT-quoted strategy) via
// the pricer. It caches resolved routes across every transaction folded in
// one history()/summary() call, so a currency that recurs across many
// transactions isn't re-resolved per transaction (a resolved-but-missing
// route is cached too — a currency the feed simply doesn't carry is only
// looked up, and logged, once).
type costConverter struct {
	p      portfolio
	routes map[string]*pricing.Route // nil value = resolved, no route exists
}

func (p portfolio) newCostConverter() *costConverter {
	return &costConverter{p: p, routes: make(map[string]*pricing.Route)}
}

// cost is the quote-equivalent cash moved by transaction t: positive when
// cash was spent (base was bought), negative when cash was received (base
// was sold, reducing basis directly — a noted v1 limitation). A transaction
// touching neither side of p.base — matched exactly, unlike quote; a
// different asset is never treated as a differently-denominated cost —
// contributes nothing. ok is false when the cash leg's symbol couldn't be
// converted into quote (no route, or no price within the tx-staleness
// window at t's timestamp); the caller must then treat its running invested
// total as unknown from that point on, not silently short it by treating
// the unconverted amount as zero.
func (c *costConverter) cost(t model.Transaction) (money.Money, bool) {
	switch {
	case t.GainedSymbol == c.p.base: // bought base; the spent leg is the cash paid
		return c.cashLeg(t.SpentSymbol, t.SpentAmount, t.Timestamp)
	case t.SpentSymbol == c.p.base: // sold base; the gained leg is the cash received
		amt, ok := c.cashLeg(t.GainedSymbol, t.GainedAmount, t.Timestamp)
		return amt.Neg(), ok
	default:
		return money.Zero, true // neither leg touches base: genuinely unrelated
	}
}

// cashLeg converts amount (denominated in symbol) into quote currency as of
// at. Already being in quote needs no lookup or pricer call at all.
func (c *costConverter) cashLeg(symbol string, amount money.Money, at time.Time) (money.Money, bool) {
	if symbol == c.p.quote {
		return amount, true
	}

	route, tried := c.routes[symbol]
	if !tried {
		r, found, err := c.p.pricer.ResolveRoute(symbol, c.p.quote)
		switch {
		case err != nil:
			log.Printf("strategy: resolving %s->%s route for a transaction's cost: %v", symbol, c.p.quote, err)
		case !found:
			log.Printf("strategy: no %s->%s route; a transaction's cost can't be converted, invested totals from this point are unknown", symbol, c.p.quote)
		}
		if err != nil || !found {
			c.routes[symbol] = nil
			return money.Zero, false
		}
		c.routes[symbol] = r
		route = r
	}
	if route == nil {
		return money.Zero, false // cached miss from an earlier transaction
	}

	price, ok, err := c.p.pricer.PriceAtRoute(route, at, c.p.txStaleness)
	if err != nil {
		log.Printf("strategy: pricing %s->%s at %v: %v", symbol, c.p.quote, at, err)
		return money.Zero, false
	}
	if !ok {
		log.Printf("strategy: no %s->%s price within the staleness window at %v; a transaction's cost can't be converted, invested totals from this point are unknown", symbol, c.p.quote, at)
		return money.Zero, false
	}
	return amount.Mul(money.FromPrice(price)), true
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

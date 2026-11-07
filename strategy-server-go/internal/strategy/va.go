package strategy

import (
	"time"

	"strategy-server-go/internal/money"
)

// VA tops the portfolio up to a target that grows by increment every
// interval, modeled as growing linearly between interval boundaries (see
// targetAt). It owns this math end-to-end — portfolio supplies only the
// universal actual-value/invested curves, which VA merges its own target
// and recommended-contribution curves into before display conversion runs.
type VA struct {
	pf portfolio
}

// GetHistory returns invested/actual (universal) plus VA's own target and
// recommended-contribution curves, in the requested display currency.
func (v VA) GetHistory(display string, rangeFrom, now time.Time, maxPoints int) (*History, error) {
	dates := v.pf.dates(rangeFrom, now, maxPoints)
	h, err := v.pf.history(dates)
	if err != nil {
		return nil, err
	}

	for i := range h.Points {
		pt := &h.Points[i]
		target := v.targetAt(pt.Date)
		pt.Values["target"] = target
		// recommended_contribution needs a priced "actual"; absent when the
		// price itself was missing at this point (pt.Missing).
		if actual, ok := pt.Values["actual"]; ok {
			pt.Values["recommended_contribution"] = target.Sub(actual)
		}
	}
	h.Series = append(h.Series,
		SeriesMeta{Key: "recommended_contribution", Label: "Recommended Contribution"},
		SeriesMeta{Key: "target", Label: "Target"},
	)

	if err := v.pf.toDisplay(h, display); err != nil {
		return nil, err
	}
	return h, nil
}

// GetSummary returns total-invested/current-value/gain-loss (universal)
// plus VA's own target and next-recommended-contribution.
func (v VA) GetSummary(display string, now time.Time) (*Summary, error) {
	sum, err := v.pf.summary(now)
	if err != nil {
		return nil, err
	}

	d := v.clampToStart(now)
	sum.Values["target"] = v.targetAt(d)
	// next_recommended_contribution needs a live current value; absent
	// when the price is stale (sum.PriceStale), same as portfolio.summary.
	if cv, ok := sum.Values["current_value"]; ok {
		_, _, next := intervalBounds(v.pf.start, v.pf.interval, d)
		sum.Values["next_recommended_contribution"] = v.targetAt(next).Sub(cv)
	}

	if err := v.pf.toDisplaySummary(sum, display); err != nil {
		return nil, err
	}
	return sum, nil
}

func (v VA) DisplayCurrencies() ([]string, string, error) {
	return v.pf.displayCurrencies()
}

// targetAt returns the target portfolio value at an arbitrary date d,
// modeled as a straight line expected = grad*(d-start) + b: grad is
// increment/interval-length and b is the value already due at the
// strategy's own start date. b is one full increment — the user is
// assumed to invest immediately at start rather than wait a full cycle —
// so the line reads increment at start, 2*increment at start+1*interval,
// and so on, agreeing with the discrete step-count target
// (stepCount*increment) at every boundary while interpolating linearly in
// between. Every date in this system is day-aligned (see sampleDates), so
// growth is computed in whole days — increment is multiplied by the
// elapsed day count before dividing by the period's day length, rather
// than pre-dividing into a per-day rate, to avoid losing precision in that
// intermediate ratio. Dates before start return 0.
func (v VA) targetAt(d time.Time) money.Money {
	start, interval, increment := v.pf.start, v.pf.interval, v.pf.increment
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

// clampToStart treats a date before the strategy's own start as start
// itself: intervalBounds assumes d is at-or-after start, and a strategy
// that hasn't started yet still has its first contribution one full
// increment ahead (targetAt's "invest immediately at start" convention),
// not a boundary in the past. Only the summary panel's "now" can precede
// start (a future-dated strategy); history's own dates never do (see
// portfolio.dates), so only GetSummary needs this.
func (v VA) clampToStart(d time.Time) time.Time {
	if d.Before(v.pf.start) {
		return v.pf.start
	}
	return d
}

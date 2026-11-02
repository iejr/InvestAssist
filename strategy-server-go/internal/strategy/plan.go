package strategy

import (
	"time"

	"strategy-server-go/internal/model"
	"strategy-server-go/internal/money"
)

// Plan is the strategy-type-specific behavior GetHistory/GetSummary call out
// to. Everything else — actual portfolio value, invested amount, date
// sampling — is universal across every strategy type and lives directly on
// Service, because it never varies by type; putting non-varying behavior
// behind an interface would just force every future Plan to write the same
// body.
//
// This is composition (Service holds a Plan), not inheritance: Go struct
// embedding doesn't give virtual dispatch the way a C++/Java base class
// does — an embedded type's own methods never see a outer type's overrides
// when they call each other — so "shared base behavior with overridable
// steps" has to be modeled as one concrete engine calling out to a small
// interface it holds, which is exactly the GoF Strategy pattern (same word
// as our domain's VA/DCA "strategies", unrelated concept).
type Plan interface {
	// Recommended is the recommended contribution as of date d, given the
	// portfolio's actual value then (nil when the price at d is unknown).
	// ok is false when the plan can't answer without knowing actual — VA
	// needs it (target - actual); DCA's flat increment never does, so DCA
	// is always ok regardless of whether actual is nil. Used for the
	// per-point history curve.
	Recommended(d time.Time, actual *money.Money) (rec money.Money, ok bool)

	// NextRecommended is the recommended contribution for the upcoming
	// contribution event after date d, given the strategy's current
	// portfolio value (nil when unknown). Same ok semantics as Recommended.
	// Used for the summary panel's forward-looking figure.
	NextRecommended(d time.Time, current *money.Money) (rec money.Money, ok bool)

	// Extras returns named strategy-specific values as of date d, for
	// charting (History) and the summary panel — e.g. VA's "target". Nil
	// for a plan with none (e.g. DCA): the target/expected-portfolio curve
	// is a VA-specific auxiliary concept, not a universal one.
	Extras(d time.Time) map[string]money.Money

	// ExtraLabels names every key Extras may return, with a display label,
	// for building History's series manifest. Static (doesn't depend on a
	// date) — e.g. VA always returns {"target": "Target"}; DCA returns nil.
	ExtraLabels() map[string]string
}

// NewPlan builds the Plan for a loaded strategy row. strat.Type is validated
// at creation time (see internal/api), so an unknown type here would be a
// DB-invariant violation, not a runtime condition callers need to handle.
func NewPlan(strat model.Strategy) Plan {
	switch strat.Type {
	case model.StrategyTypeVA:
		return vaPlan{start: strat.StartDate, interval: strat.Interval, increment: strat.Increment}
	case model.StrategyTypeDCA:
		return dcaPlan{increment: strat.Increment}
	default:
		panic("strategy: unknown strategy type " + string(strat.Type))
	}
}

// vaPlan tops the portfolio up to a target that grows by increment every
// interval, modeled as growing linearly between interval boundaries (see
// ExpectedTarget).
type vaPlan struct {
	start     time.Time
	interval  model.Interval
	increment money.Money
}

func (p vaPlan) Recommended(d time.Time, actual *money.Money) (money.Money, bool) {
	if actual == nil {
		return money.Zero, false
	}
	target := ExpectedTarget(p.start, p.interval, p.increment, d)
	return target.Sub(*actual), true
}

func (p vaPlan) NextRecommended(d time.Time, current *money.Money) (money.Money, bool) {
	if current == nil {
		return money.Zero, false
	}
	_, _, next := intervalBounds(p.start, p.interval, p.clampToStart(d))
	nextTarget := ExpectedTarget(p.start, p.interval, p.increment, next)
	return nextTarget.Sub(*current), true
}

func (p vaPlan) Extras(d time.Time) map[string]money.Money {
	target := ExpectedTarget(p.start, p.interval, p.increment, p.clampToStart(d))
	return map[string]money.Money{"target": target}
}

func (p vaPlan) ExtraLabels() map[string]string {
	return map[string]string{"target": "Target"}
}

// clampToStart treats a date before the strategy's own start as start
// itself: intervalBounds assumes d is at-or-after start, and a strategy
// that hasn't started yet still has its first contribution one full
// increment ahead (ExpectedTarget's "invest immediately at start"
// convention), not a boundary in the past.
func (p vaPlan) clampToStart(d time.Time) time.Time {
	if d.Before(p.start) {
		return p.start
	}
	return d
}

// dcaPlan contributes a fixed increment every interval, independent of the
// portfolio's actual value.
type dcaPlan struct {
	increment money.Money
}

func (p dcaPlan) Recommended(time.Time, *money.Money) (money.Money, bool) {
	return p.increment, true
}

func (p dcaPlan) NextRecommended(time.Time, *money.Money) (money.Money, bool) {
	return p.increment, true
}

func (p dcaPlan) Extras(time.Time) map[string]money.Money {
	return nil
}

func (p dcaPlan) ExtraLabels() map[string]string {
	return nil
}

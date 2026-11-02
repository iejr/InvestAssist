package strategy

import (
	"testing"
	"time"

	"strategy-server-go/internal/model"
	"strategy-server-go/internal/money"

	"github.com/shopspring/decimal"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func mustMoney(t *testing.T, s string) money.Money {
	t.Helper()
	v, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("decimal.NewFromString(%q): %v", s, err)
	}
	return v
}

func TestSampleDates_FitsWithinMaxPoints_ReturnsEveryDay(t *testing.T) {
	start := day(2026, 1, 1)
	end := day(2026, 1, 10) // 10 days, well within maxPoints
	got := sampleDates(start, end, 50)

	if len(got) != 10 {
		t.Fatalf("len = %d, want 10", len(got))
	}
	for i, d := range got {
		want := start.AddDate(0, 0, i)
		if !d.Equal(want) {
			t.Fatalf("got[%d] = %v, want %v", i, d, want)
		}
	}
}

func TestSampleDates_BoundsPointCount_AlwaysPinsEndpoints(t *testing.T) {
	start := day(2020, 1, 1)
	end := day(2026, 1, 1) // ~6 years, way more days than maxPoints
	maxPoints := 50

	got := sampleDates(start, end, maxPoints)

	if len(got) > maxPoints {
		t.Fatalf("len = %d, want <= %d", len(got), maxPoints)
	}
	if !got[0].Equal(start) {
		t.Fatalf("first point = %v, want %v (start)", got[0], start)
	}
	if !got[len(got)-1].Equal(end) {
		t.Fatalf("last point = %v, want %v (end)", got[len(got)-1], end)
	}
	// Strictly increasing, so the chart's x-axis is monotonic.
	for i := 1; i < len(got); i++ {
		if !got[i].After(got[i-1]) {
			t.Fatalf("dates not strictly increasing at index %d: %v then %v", i, got[i-1], got[i])
		}
	}
}

func TestSampleDates_SingleDay(t *testing.T) {
	d := day(2026, 3, 15)
	got := sampleDates(d, d, 50)
	if len(got) != 1 || !got[0].Equal(d) {
		t.Fatalf("got %v, want single point %v", got, d)
	}
}

func TestSampleDates_StartAfterEnd_ReturnsNil(t *testing.T) {
	start := day(2026, 6, 1)
	end := day(2026, 1, 1) // strategy hasn't started yet relative to "end"
	got := sampleDates(start, end, 50)
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestSampleDates_MaxPointsBelowTwo_ClampsToTwo(t *testing.T) {
	start := day(2026, 1, 1)
	end := day(2026, 1, 1).AddDate(0, 0, 100)
	got := sampleDates(start, end, 0)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (clamped)", len(got))
	}
	if !got[0].Equal(start) || !got[1].Equal(end) {
		t.Fatalf("got %v, want [%v, %v]", got, start, end)
	}
}

func TestExpectedTarget_BeforeStart_IsZero(t *testing.T) {
	start := day(2026, 1, 8)
	got := ExpectedTarget(start, model.IntervalWeekly, money.FromInt(100), day(2026, 1, 1))
	if !got.Equal(money.Zero) {
		t.Fatalf("got %v, want 0", got)
	}
}

func TestExpectedTarget_Weekly_AgreesWithStepFunctionAtBoundaries(t *testing.T) {
	start := day(2026, 1, 1)
	cases := []struct {
		d    time.Time
		want money.Money
	}{
		{day(2026, 1, 1), money.FromInt(100)},  // start: 1st step already due
		{day(2026, 1, 8), money.FromInt(200)},  // 2nd boundary
		{day(2026, 1, 15), money.FromInt(300)}, // 3rd boundary
	}
	for _, c := range cases {
		got := ExpectedTarget(start, model.IntervalWeekly, money.FromInt(100), c.d)
		if !got.Equal(c.want) {
			t.Fatalf("ExpectedTarget(%v) = %v, want %v", c.d, got, c.want)
		}
	}
}

func TestExpectedTarget_Weekly_InterpolatesLinearlyMidInterval(t *testing.T) {
	start := day(2026, 1, 1)
	// 3 of the 7 days into the first week (dates are day-aligned, so the
	// elapsed fraction is computed in whole days, not sub-day durations).
	d := start.AddDate(0, 0, 3)
	got := ExpectedTarget(start, model.IntervalWeekly, money.FromInt(100), d)
	// (1 + 3/7) * 100
	want := money.FromInt(100).Mul(money.FromInt(3)).Div(money.FromInt(7)).Add(money.FromInt(100))
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpectedTarget_Weekly_MonotonicNoJumpBackwards(t *testing.T) {
	start := day(2026, 1, 1)
	prev := ExpectedTarget(start, model.IntervalWeekly, money.FromInt(100), start)
	for i := 1; i <= 30; i++ {
		d := start.AddDate(0, 0, i)
		got := ExpectedTarget(start, model.IntervalWeekly, money.FromInt(100), d)
		if got.LessThan(prev) {
			t.Fatalf("target decreased at day %d: %v -> %v", i, prev, got)
		}
		prev = got
	}
}

func TestExpectedTarget_Monthly_AgreesWithStepFunctionAtBoundaries(t *testing.T) {
	start := day(2026, 1, 31) // exercises AddDate day-overflow normalization
	cases := []struct {
		d    time.Time
		want money.Money
	}{
		{day(2026, 1, 31), money.FromInt(50)},
		{day(2026, 3, 3), money.FromInt(100)}, // Feb 31 normalizes to Mar 3
		{day(2026, 3, 31), money.FromInt(150)},
	}
	for _, c := range cases {
		got := ExpectedTarget(start, model.IntervalMonthly, money.FromInt(50), c.d)
		if !got.Equal(c.want) {
			t.Fatalf("ExpectedTarget(%v) = %v, want %v", c.d, got, c.want)
		}
	}
}

func TestExpectedTarget_NoFloatDrift_AcrossManyFractionalIncrements(t *testing.T) {
	// A classic float64 trap: 0.1 has no exact binary representation, so
	// summing/scaling it repeatedly drifts. Decimal arithmetic must not.
	start := day(2026, 1, 1)
	increment := mustMoney(t, "0.1")
	d := start.AddDate(0, 0, 3) // 3/7ths into the first week
	got := ExpectedTarget(start, model.IntervalWeekly, increment, d)
	want := mustMoney(t, "0.1").Mul(mustMoney(t, "3")).Div(mustMoney(t, "7")).Add(mustMoney(t, "0.1"))
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSampleDates_NormalizesNonUTCToUTCMidnight(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*3600)
	// 2026-01-01 00:00 +08:00 is 2025-12-31 16:00 UTC — truncating in UTC
	// must not silently shift the intended calendar day's hour component
	// into a different bucket than a UTC caller would get for the same
	// instant.
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 5)

	got := sampleDates(start, end, 50)
	if len(got) != 6 {
		t.Fatalf("len = %d, want 6", len(got))
	}
	for _, d := range got {
		if d.Location() != time.UTC {
			t.Fatalf("date %v not normalized to UTC", d)
		}
		if h, m, s := d.Clock(); h != 0 || m != 0 || s != 0 {
			t.Fatalf("date %v not at UTC midnight", d)
		}
	}
}

// --- Plan ---

func TestNewPlan_VA_RecommendedIsTargetMinusActual(t *testing.T) {
	strat := model.Strategy{
		Type:      model.StrategyTypeVA,
		StartDate: day(2026, 1, 1),
		Interval:  model.IntervalWeekly,
		Increment: money.FromInt(100),
	}
	plan := NewPlan(strat)

	actual := money.FromInt(40)
	rec, ok := plan.Recommended(day(2026, 1, 1), &actual)
	if !ok {
		t.Fatalf("Recommended: ok = false, want true")
	}
	want := money.FromInt(60) // target 100 - actual 40
	if !rec.Equal(want) {
		t.Fatalf("Recommended = %v, want %v", rec, want)
	}

	if _, ok := plan.Recommended(day(2026, 1, 1), nil); ok {
		t.Fatalf("Recommended with nil actual: ok = true, want false (VA needs a price)")
	}
}

func TestNewPlan_DCA_RecommendedIsFlatIncrementRegardlessOfActual(t *testing.T) {
	strat := model.Strategy{
		Type:      model.StrategyTypeDCA,
		StartDate: day(2026, 1, 1),
		Interval:  model.IntervalWeekly,
		Increment: money.FromInt(25),
	}
	plan := NewPlan(strat)

	actual := money.FromInt(9999)
	for _, in := range []*money.Money{&actual, nil} {
		rec, ok := plan.Recommended(day(2026, 1, 1), in)
		if !ok {
			t.Fatalf("Recommended: ok = false, want true")
		}
		if !rec.Equal(money.FromInt(25)) {
			t.Fatalf("Recommended = %v, want 25", rec)
		}
	}
}

func TestNewPlan_VA_ExtrasHasTarget_DCA_HasNone(t *testing.T) {
	va := NewPlan(model.Strategy{
		Type: model.StrategyTypeVA, StartDate: day(2026, 1, 1),
		Interval: model.IntervalWeekly, Increment: money.FromInt(100),
	})
	extras := va.Extras(day(2026, 1, 1))
	if got, ok := extras["target"]; !ok || !got.Equal(money.FromInt(100)) {
		t.Fatalf("VA Extras[target] = %v, ok=%v, want 100, true", got, ok)
	}

	dca := NewPlan(model.Strategy{
		Type: model.StrategyTypeDCA, StartDate: day(2026, 1, 1),
		Interval: model.IntervalWeekly, Increment: money.FromInt(100),
	})
	if extras := dca.Extras(day(2026, 1, 1)); len(extras) != 0 {
		t.Fatalf("DCA Extras = %v, want none", extras)
	}
	if labels := dca.ExtraLabels(); len(labels) != 0 {
		t.Fatalf("DCA ExtraLabels = %v, want none", labels)
	}
}

func TestNewPlan_VA_NextRecommended_UsesNextBoundaryTarget(t *testing.T) {
	strat := model.Strategy{
		Type:      model.StrategyTypeVA,
		StartDate: day(2026, 1, 1),
		Interval:  model.IntervalWeekly,
		Increment: money.FromInt(100),
	}
	plan := NewPlan(strat)

	current := money.FromInt(40)
	rec, ok := plan.NextRecommended(day(2026, 1, 1), &current)
	if !ok {
		t.Fatalf("NextRecommended: ok = false, want true")
	}
	// Next boundary after start (2026-01-08) target is 2*increment = 200.
	want := money.FromInt(160)
	if !rec.Equal(want) {
		t.Fatalf("NextRecommended = %v, want %v", rec, want)
	}
}

func TestNewPlan_VA_NextRecommended_BeforeStart_ClampsToStart(t *testing.T) {
	strat := model.Strategy{
		Type:      model.StrategyTypeVA,
		StartDate: day(2026, 2, 1),
		Interval:  model.IntervalWeekly,
		Increment: money.FromInt(100),
	}
	plan := NewPlan(strat)

	current := money.Zero
	rec, ok := plan.NextRecommended(day(2026, 1, 1), &current) // before start
	if !ok {
		t.Fatalf("NextRecommended: ok = false, want true")
	}
	// Clamped to start: next boundary after start is start+7d, target 200.
	if !rec.Equal(money.FromInt(200)) {
		t.Fatalf("NextRecommended = %v, want 200", rec)
	}

	extras := plan.Extras(day(2026, 1, 1))
	if !extras["target"].Equal(money.FromInt(100)) {
		t.Fatalf("Extras[target] = %v, want 100 (clamped to start)", extras["target"])
	}
}

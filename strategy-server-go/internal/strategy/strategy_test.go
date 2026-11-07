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

func TestStepCountAt_BeforeStart_IsZero(t *testing.T) {
	start := day(2026, 1, 8)
	if got := stepCountAt(start, model.IntervalWeekly, day(2026, 1, 1)); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestStepCountAt_Weekly_AtBoundaries(t *testing.T) {
	start := day(2026, 1, 1)
	cases := []struct {
		d    time.Time
		want int
	}{
		{day(2026, 1, 1), 1},  // start: 1st step already due
		{day(2026, 1, 8), 2},  // 2nd boundary
		{day(2026, 1, 15), 3}, // 3rd boundary
	}
	for _, c := range cases {
		if got := stepCountAt(start, model.IntervalWeekly, c.d); got != c.want {
			t.Fatalf("stepCountAt(%v) = %d, want %d", c.d, got, c.want)
		}
	}
}

// --- VA ---

func newVA(start time.Time, interval model.Interval, increment money.Money) VA {
	return VA{pf: portfolio{start: start, interval: interval, increment: increment}}
}

func TestVA_TargetAt_BeforeStart_IsZero(t *testing.T) {
	start := day(2026, 1, 8)
	va := newVA(start, model.IntervalWeekly, money.FromInt(100))
	got := va.targetAt(day(2026, 1, 1))
	if !got.Equal(money.Zero) {
		t.Fatalf("got %v, want 0", got)
	}
}

func TestVA_TargetAt_Weekly_AgreesWithStepFunctionAtBoundaries(t *testing.T) {
	start := day(2026, 1, 1)
	va := newVA(start, model.IntervalWeekly, money.FromInt(100))
	cases := []struct {
		d    time.Time
		want money.Money
	}{
		{day(2026, 1, 1), money.FromInt(100)},  // start: 1st step already due
		{day(2026, 1, 8), money.FromInt(200)},  // 2nd boundary
		{day(2026, 1, 15), money.FromInt(300)}, // 3rd boundary
	}
	for _, c := range cases {
		got := va.targetAt(c.d)
		if !got.Equal(c.want) {
			t.Fatalf("targetAt(%v) = %v, want %v", c.d, got, c.want)
		}
	}
}

func TestVA_TargetAt_Weekly_InterpolatesLinearlyMidInterval(t *testing.T) {
	start := day(2026, 1, 1)
	va := newVA(start, model.IntervalWeekly, money.FromInt(100))
	// 3 of the 7 days into the first week (dates are day-aligned, so the
	// elapsed fraction is computed in whole days, not sub-day durations).
	d := start.AddDate(0, 0, 3)
	got := va.targetAt(d)
	// (1 + 3/7) * 100
	want := money.FromInt(100).Mul(money.FromInt(3)).Div(money.FromInt(7)).Add(money.FromInt(100))
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestVA_TargetAt_Weekly_MonotonicNoJumpBackwards(t *testing.T) {
	start := day(2026, 1, 1)
	va := newVA(start, model.IntervalWeekly, money.FromInt(100))
	prev := va.targetAt(start)
	for i := 1; i <= 30; i++ {
		d := start.AddDate(0, 0, i)
		got := va.targetAt(d)
		if got.LessThan(prev) {
			t.Fatalf("target decreased at day %d: %v -> %v", i, prev, got)
		}
		prev = got
	}
}

func TestVA_TargetAt_Monthly_AgreesWithStepFunctionAtBoundaries(t *testing.T) {
	start := day(2026, 1, 31) // exercises AddDate day-overflow normalization
	va := newVA(start, model.IntervalMonthly, money.FromInt(50))
	cases := []struct {
		d    time.Time
		want money.Money
	}{
		{day(2026, 1, 31), money.FromInt(50)},
		{day(2026, 3, 3), money.FromInt(100)}, // Feb 31 normalizes to Mar 3
		{day(2026, 3, 31), money.FromInt(150)},
	}
	for _, c := range cases {
		got := va.targetAt(c.d)
		if !got.Equal(c.want) {
			t.Fatalf("targetAt(%v) = %v, want %v", c.d, got, c.want)
		}
	}
}

func TestVA_TargetAt_NoFloatDrift_AcrossManyFractionalIncrements(t *testing.T) {
	// A classic float64 trap: 0.1 has no exact binary representation, so
	// summing/scaling it repeatedly drifts. Decimal arithmetic must not.
	start := day(2026, 1, 1)
	increment := mustMoney(t, "0.1")
	va := newVA(start, model.IntervalWeekly, increment)
	d := start.AddDate(0, 0, 3) // 3/7ths into the first week
	got := va.targetAt(d)
	want := mustMoney(t, "0.1").Mul(mustMoney(t, "3")).Div(mustMoney(t, "7")).Add(mustMoney(t, "0.1"))
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestVA_ClampToStart_AtOrAfterStart_Unchanged(t *testing.T) {
	start := day(2026, 1, 1)
	va := newVA(start, model.IntervalWeekly, money.FromInt(100))
	d := day(2026, 1, 15)
	if got := va.clampToStart(d); !got.Equal(d) {
		t.Fatalf("got %v, want %v (unchanged)", got, d)
	}
}

func TestVA_ClampToStart_BeforeStart_ClampsToStart(t *testing.T) {
	start := day(2026, 2, 1)
	va := newVA(start, model.IntervalWeekly, money.FromInt(100))
	if got := va.clampToStart(day(2026, 1, 1)); !got.Equal(start) {
		t.Fatalf("got %v, want %v (start)", got, start)
	}
}

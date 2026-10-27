package strategy

import (
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
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

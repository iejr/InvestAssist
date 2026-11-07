package strategy

import (
	"time"

	"strategy-server-go/internal/model"
)

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

// stepCountAt is the number of whole contribution steps due at-or-before d:
// 0 strictly before start, 1 at start (the user is assumed to invest
// immediately at start rather than wait a full cycle), 2 at the first
// boundary after start, and so on. Shared by every strategy type — it's the
// raw discrete cadence, independent of any type's own valuation math.
func stepCountAt(start time.Time, interval model.Interval, d time.Time) int {
	if d.Before(start) {
		return 0
	}
	step, _, _ := intervalBounds(start, interval, d)
	return step + 1
}

// maxStaleness is one interval-step by default (weekly ⇒ ~1 week, monthly ⇒
// ~1 month), overridable via config.
func maxStaleness(interval model.Interval, override time.Duration) time.Duration {
	if override > 0 {
		return override
	}
	if interval == model.IntervalWeekly {
		return 7 * 24 * time.Hour
	}
	return 31 * 24 * time.Hour
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

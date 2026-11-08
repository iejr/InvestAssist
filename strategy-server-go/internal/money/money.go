// Package money is this service's fixed-precision representation for every
// monetary value it owns outright — Increment, transaction amounts, and
// everything computed from them (targets, recommendations, gain/loss).
// Backed by shopspring/decimal so chained arithmetic (multiply then divide,
// running sums across many transactions) never accumulates the binary
// float64 rounding error a plain float would.
package money

import "github.com/shopspring/decimal"

// Money is a plain alias for decimal.Decimal, so its full arithmetic API
// (Add, Sub, Mul, Div, Round, ...) is usable directly under this package's
// name.
type Money = decimal.Decimal

// Zero is the additive identity.
var Zero = decimal.Zero

// FromInt builds a Money from a small integer (e.g. a step count) for use in
// Money arithmetic.
func FromInt(n int) Money {
	return decimal.NewFromInt(int64(n))
}

// FromPrice converts a float64 price read from market-feed's double-
// precision storage (internal/pricing) into Money. This is the one
// chokepoint where a float-sourced value enters this service's otherwise-
// exact arithmetic: market-feed's own storage is still double precision
// today, so some upstream imprecision is unavoidable here until that's
// migrated separately, in market-feed's own repo. Uses the shortest
// round-trip decimal string (decimal.NewFromFloat), not the literal binary
// expansion — feed prices are decimal-ish by nature, so this recovers the
// value actually quoted rather than reproducing float noise.
func FromPrice(price float64) Money {
	return decimal.NewFromFloat(price)
}

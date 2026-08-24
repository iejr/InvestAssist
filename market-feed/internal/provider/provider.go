package provider

import (
	"context"
	"time"

	"market-feed/internal/model"
)

// PriceEvent is the normalized, source-agnostic representation of a single
// observed trade/price. Downstream consumers never learn whether it came from
// WebSocket, REST, or another exchange.
type PriceEvent struct {
	Base      string       // e.g. "BTC"
	Quote     string       // e.g. "USDT"
	Price     float64      // price of Base expressed in Quote
	Volume    float64      // trade quantity in Base units (0 if unknown)
	Timestamp time.Time    // exchange event time
	Source    model.Source // e.g. "binance"
}

// Providers expose capabilities as small, separately-satisfiable interfaces
// rather than one monolithic type. A driver implements only what it can:
// Binance is a Streamer + OHLCFetcher; Kraken is an OHLCFetcher + TickerFetcher
// (and could add Streamer later). Runners depend on the narrowest capability
// they need, and the app validates a job against the driver's capabilities at
// startup rather than failing at runtime.
//
// The app speaks canonical (base, quote); each driver translates to and from
// its own native symbol scheme internally (see per-provider normalization).

// Streamer pushes normalized price events for the given native symbols until the
// context is cancelled. Implementations own reconnection; the returned channel
// stays open across reconnects and is closed only when ctx ends.
type Streamer interface {
	Stream(ctx context.Context, symbols []string) (<-chan PriceEvent, error)
}

// OHLCFetcher pulls historical candles for one (base, quote) edge at one
// interval over [start, end). Used by both backfill (one-shot range) and, later,
// scheduled polling (recurring recent window) — the driver code is shared even
// though those remain distinct jobs.
type OHLCFetcher interface {
	FetchOHLC(ctx context.Context, base, quote string, iv model.Interval, start, end time.Time) ([]model.PriceCandle, error)
}

// TickerFetcher pulls the current spot price for one (base, quote) edge.
type TickerFetcher interface {
	Ticker(ctx context.Context, base, quote string) (float64, time.Time, error)
}

// Compile-time capability assertions: fail the build here (not at a far-away
// call site) if a driver ever stops satisfying the capability it advertises.
var (
	_ Streamer      = (*BinanceWS)(nil)
	_ OHLCFetcher   = (*BinanceREST)(nil)
	_ OHLCFetcher   = (*KrakenREST)(nil)
	_ TickerFetcher = (*KrakenREST)(nil)
)

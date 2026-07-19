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

// MarketProvider streams normalized price events for the given symbols until
// the context is cancelled. Implementations own reconnection; the returned
// channel stays open across reconnects and is closed only when ctx ends.
type MarketProvider interface {
	Stream(ctx context.Context, symbols []string) (<-chan PriceEvent, error)
}

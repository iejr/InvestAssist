package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"market-feed/internal/model"
	"market-feed/internal/normalize"

	"github.com/gorilla/websocket"
)

// BinanceWS streams trade events from Binance's combined websocket endpoint.
// It owns reconnection: the event channel stays open across drops and closes
// only when the context is cancelled.
type BinanceWS struct {
	wsBase string
}

// NewBinanceWS constructs a provider against the given websocket base URL
// (e.g. "wss://stream.binance.com:9443").
func NewBinanceWS(wsBase string) *BinanceWS {
	return &BinanceWS{wsBase: strings.TrimRight(wsBase, "/")}
}

// binanceCombinedMsg is the envelope Binance sends on the combined stream.
type binanceCombinedMsg struct {
	Stream string          `json:"stream"`
	Data   binanceTradeMsg `json:"data"`
}

// binanceTradeMsg is the payload of an @trade event. Only the fields we use
// are decoded.
type binanceTradeMsg struct {
	Event     string `json:"e"` // "trade"
	EventTime int64  `json:"E"` // ms since epoch. Declared so Go's case-insensitive
	// JSON matching doesn't collide the numeric "E" key with the string "e" field.
	Symbol    string `json:"s"` // "BTCUSDT"
	TradeID   int64  `json:"t"` // trade id. Declared so the numeric "t" key gets an
	// exact match instead of colliding case-insensitively with "T" below.
	Price     string `json:"p"` // decimal string
	Quantity  string `json:"q"` // decimal string
	TradeTime int64  `json:"T"` // ms since epoch (trade time)
}

func (b *BinanceWS) Stream(ctx context.Context, symbols []string) (<-chan PriceEvent, error) {
	if len(symbols) == 0 {
		return nil, fmt.Errorf("binance ws: no symbols configured")
	}

	streams := make([]string, 0, len(symbols))
	for _, s := range symbols {
		streams = append(streams, strings.ToLower(s)+"@trade")
	}
	url := fmt.Sprintf("%s/stream?streams=%s", b.wsBase, strings.Join(streams, "/"))

	out := make(chan PriceEvent, 1024)

	go func() {
		defer close(out)
		backoff := time.Second
		const maxBackoff = 30 * time.Second

		for ctx.Err() == nil {
			if err := b.runOnce(ctx, url, out); err != nil && ctx.Err() == nil {
				log.Printf("binance ws: connection error: %v; reconnecting in %s", err, backoff)
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				if backoff *= 2; backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}
			// Clean disconnect without ctx cancel: reset backoff and retry.
			backoff = time.Second
		}
	}()

	return out, nil
}

// runOnce holds a single websocket connection open, forwarding trade events to
// out until the connection fails or ctx is cancelled.
func (b *BinanceWS) runOnce(ctx context.Context, url string, out chan<- PriceEvent) error {
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		// On a failed upgrade gorilla still returns the HTTP response; surface
		// its status and a snippet of the body so geo-blocks (451), rate limits
		// (429), and endpoint issues are diagnosable instead of "bad handshake".
		if resp != nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			return fmt.Errorf("dial: %w (http %d: %s)", err, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	log.Printf("binance ws: connected to %s", url)

	// Close the connection when ctx is cancelled so the blocking ReadMessage
	// below returns promptly.
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	// Binance sends a ping every ~3 min and expects a pong; gorilla replies to
	// pings automatically. We also bound reads so a dead peer is detected.
	conn.SetReadDeadline(time.Now().Add(10 * time.Minute))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(10 * time.Minute))
		return nil
	})

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}

		var msg binanceCombinedMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("binance ws: bad message: %v", err)
			continue
		}
		if msg.Data.Event != "trade" {
			continue
		}

		ev, ok := toPriceEvent(msg.Data)
		if !ok {
			continue
		}

		select {
		case out <- ev:
		case <-ctx.Done():
			return nil
		}
	}
}

// toPriceEvent normalizes a raw Binance trade into a PriceEvent. Returns
// ok=false for unparseable prices or unrecognized symbols.
func toPriceEvent(t binanceTradeMsg) (PriceEvent, bool) {
	base, quote, ok := normalize.Symbol(t.Symbol)
	if !ok {
		return PriceEvent{}, false
	}
	price, err := strconv.ParseFloat(t.Price, 64)
	if err != nil {
		return PriceEvent{}, false
	}
	qty, _ := strconv.ParseFloat(t.Quantity, 64)

	return PriceEvent{
		Base:      base,
		Quote:     quote,
		Price:     price,
		Volume:    qty,
		Timestamp: time.UnixMilli(t.TradeTime).UTC(),
		Source:    model.SourceBinance,
	}, true
}

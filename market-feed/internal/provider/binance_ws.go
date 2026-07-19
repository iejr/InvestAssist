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
		const stableThreshold = 2 * time.Minute

		for ctx.Err() == nil {
			start := time.Now()
			err := b.runOnce(ctx, url, out)
			if ctx.Err() != nil {
				return
			}
			// A connection that stayed healthy for a while means the endpoint
			// is fine; treat the drop as isolated and reset backoff so we
			// reconnect promptly instead of at a stale (possibly maxed) delay.
			if time.Since(start) > stableThreshold {
				backoff = time.Second
			}
			if err != nil {
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

	// Detect a dead peer: require some frame (data, ping, or pong) within the
	// read window. Binance pings us periodically and also sends trades often,
	// so any healthy connection refreshes this deadline continuously.
	const readWait = 3 * time.Minute
	resetDeadline := func() { _ = conn.SetReadDeadline(time.Now().Add(readWait)) }
	resetDeadline()
	// gorilla auto-replies to pings; we just refresh the deadline on each.
	conn.SetPingHandler(func(appData string) error {
		resetDeadline()
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(10*time.Second))
	})
	conn.SetPongHandler(func(string) error { resetDeadline(); return nil })

	// Proactively ping so we notice a silently dropped connection quickly.
	pinger := time.NewTicker(readWait / 3)
	defer pinger.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-pinger.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
					return
				}
			}
		}
	}()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}
		// Any inbound frame proves the connection is alive.
		resetDeadline()

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

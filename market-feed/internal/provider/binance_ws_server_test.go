package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestStreamOverHTTPTest stands up a local websocket server that speaks the
// Binance combined-stream framing, points BinanceWS at it, and asserts a
// normalized PriceEvent flows out. Exercises connect -> read -> decode ->
// normalize without touching the network or a database.
func TestStreamOverHTTPTest(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sanity check the requested stream path.
		if !strings.Contains(r.URL.RawQuery, "btcusdt@trade") {
			t.Errorf("unexpected stream query: %q", r.URL.RawQuery)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(sampleTradeFrame))
		// Hold the connection briefly so the client can read before close.
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	// httptest serves http://; the dialer needs ws://.
	wsBase := "ws" + strings.TrimPrefix(srv.URL, "http")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	prov := NewBinanceWS(wsBase)
	events, err := prov.Stream(ctx, []string{"BTCUSDT"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	select {
	case ev := <-events:
		if ev.Base != "BTC" || ev.Quote != "USDT" {
			t.Errorf("base/quote = %s/%s, want BTC/USDT", ev.Base, ev.Quote)
		}
		if ev.Price != 43210.50 {
			t.Errorf("price = %v, want 43210.50", ev.Price)
		}
	case <-ctx.Done():
		t.Fatal("no event received before timeout")
	}
}

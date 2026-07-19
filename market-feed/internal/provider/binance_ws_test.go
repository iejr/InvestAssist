package provider

import (
	"encoding/json"
	"testing"
	"time"

	"market-feed/internal/model"
)

// A real Binance combined-stream @trade frame. Critically it contains both the
// string key "e" and the numeric keys "E"/"t"/"T" — Go's case-insensitive JSON
// matching previously collapsed these, so this guards that regression.
const sampleTradeFrame = `{
  "stream": "btcusdt@trade",
  "data": {
    "e": "trade",
    "E": 1699999999999,
    "s": "BTCUSDT",
    "t": 123456,
    "p": "43210.50",
    "q": "0.0125",
    "T": 1699999999000,
    "m": false
  }
}`

func TestDecodeCombinedFrame_NoKeyCollision(t *testing.T) {
	var msg binanceCombinedMsg
	if err := json.Unmarshal([]byte(sampleTradeFrame), &msg); err != nil {
		t.Fatalf("unmarshal failed (key collision regression?): %v", err)
	}
	if msg.Data.Event != "trade" {
		t.Errorf("Event = %q, want %q", msg.Data.Event, "trade")
	}
	if msg.Data.EventTime != 1699999999999 {
		t.Errorf("EventTime = %d, want 1699999999999", msg.Data.EventTime)
	}
	if msg.Data.TradeID != 123456 {
		t.Errorf("TradeID = %d, want 123456", msg.Data.TradeID)
	}
	if msg.Data.TradeTime != 1699999999000 {
		t.Errorf("TradeTime = %d, want 1699999999000", msg.Data.TradeTime)
	}
}

func TestToPriceEvent(t *testing.T) {
	var msg binanceCombinedMsg
	if err := json.Unmarshal([]byte(sampleTradeFrame), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	ev, ok := toPriceEvent(msg.Data)
	if !ok {
		t.Fatal("toPriceEvent returned ok=false for a valid trade")
	}
	if ev.Base != "BTC" || ev.Quote != "USDT" {
		t.Errorf("base/quote = %s/%s, want BTC/USDT", ev.Base, ev.Quote)
	}
	if ev.Price != 43210.50 {
		t.Errorf("price = %v, want 43210.50", ev.Price)
	}
	if ev.Volume != 0.0125 {
		t.Errorf("volume = %v, want 0.0125", ev.Volume)
	}
	if ev.Source != model.SourceBinance {
		t.Errorf("source = %q, want %q", ev.Source, model.SourceBinance)
	}
	// TradeTime (T) is used as the event timestamp, in UTC.
	want := time.UnixMilli(1699999999000).UTC()
	if !ev.Timestamp.Equal(want) {
		t.Errorf("timestamp = %v, want %v", ev.Timestamp, want)
	}
}

func TestToPriceEvent_Rejects(t *testing.T) {
	tests := []struct {
		name string
		msg  binanceTradeMsg
	}{
		{"unknown symbol", binanceTradeMsg{Symbol: "XYZ", Price: "1.0"}},
		{"unparseable price", binanceTradeMsg{Symbol: "BTCUSDT", Price: "not-a-number"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := toPriceEvent(tt.msg); ok {
				t.Errorf("toPriceEvent(%+v) ok=true, want false", tt.msg)
			}
		})
	}
}

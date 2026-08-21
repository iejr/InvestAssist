package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// krakenServer stands up a fake Kraken endpoint returning the given raw body.
func krakenServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/0/public/Ticker") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// TestKrakenTicker_ParsesLastTrade checks that the last-trade price (c[0]) is
// read even when Kraken renames the pair key (USDTUSD -> USDTZUSD).
func TestKrakenTicker_ParsesLastTrade(t *testing.T) {
	// Kraken commonly answers a differently-named key than requested.
	body := `{"error":[],"result":{"USDTZUSD":{"a":["1.0002","1","1.0"],"b":["1.0001","2","2.0"],"c":["1.00015","500.0"]}}}`
	srv := krakenServer(t, http.StatusOK, body)
	defer srv.Close()

	k := NewKrakenREST(srv.URL)
	price, ts, err := k.Ticker(context.Background(), "USDT", "USD")
	if err != nil {
		t.Fatalf("Ticker: %v", err)
	}
	if price != 1.00015 {
		t.Errorf("price = %v, want 1.00015", price)
	}
	if ts.IsZero() {
		t.Error("timestamp is zero, want now")
	}
}

// TestKrakenTicker_ErrorArray surfaces Kraken's application-level error (which
// arrives with HTTP 200 and a populated "error" array).
func TestKrakenTicker_ErrorArray(t *testing.T) {
	body := `{"error":["EQuery:Unknown asset pair"],"result":{}}`
	srv := krakenServer(t, http.StatusOK, body)
	defer srv.Close()

	k := NewKrakenREST(srv.URL)
	if _, _, err := k.Ticker(context.Background(), "XXX", "USD"); err == nil {
		t.Fatal("expected error for unknown pair, got nil")
	}
}

// TestKrakenTicker_HTTPError surfaces a non-200 with the body.
func TestKrakenTicker_HTTPError(t *testing.T) {
	srv := krakenServer(t, http.StatusTooManyRequests, "rate limited")
	defer srv.Close()

	k := NewKrakenREST(srv.URL)
	_, _, err := k.Ticker(context.Background(), "USDT", "USD")
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected 429 error, got %v", err)
	}
}

// TestKrakenTicker_RejectsNonPositive guards against a bad/zero quote.
func TestKrakenTicker_RejectsNonPositive(t *testing.T) {
	body := `{"error":[],"result":{"USDCUSD":{"c":["0.0","0.0"]}}}`
	srv := krakenServer(t, http.StatusOK, body)
	defer srv.Close()

	k := NewKrakenREST(srv.URL)
	if _, _, err := k.Ticker(context.Background(), "USDC", "USD"); err == nil {
		t.Fatal("expected error for non-positive price, got nil")
	}
}

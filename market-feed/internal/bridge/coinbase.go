package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CoinbaseREST reads stablecoin->USD spot rates from the Coinbase Exchange
// public API. We use Coinbase (not Binance) for the stable<->USD hop because it
// settles in real USD, so USDT/USD and USDC/USD are genuine observations rather
// than an assumed 1:1 peg.
type CoinbaseREST struct {
	base   string
	client *http.Client
}

// NewCoinbaseREST constructs a client against the given base URL
// (e.g. "https://api.exchange.coinbase.com").
func NewCoinbaseREST(base string) *CoinbaseREST {
	return &CoinbaseREST{
		base:   strings.TrimRight(base, "/"),
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// coinbaseTicker is the subset of GET /products/{id}/ticker we consume.
type coinbaseTicker struct {
	Price string `json:"price"`
	Time  string `json:"time"`
}

// Ticker returns the current spot price and observation time for a product id
// such as "USDT-USD". Price is expressed as quote-per-base (USD per USDT).
func (c *CoinbaseREST) Ticker(ctx context.Context, product string) (float64, time.Time, error) {
	reqURL := c.base + "/products/" + product + "/ticker"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, time.Time{}, err
	}
	// Coinbase rejects requests without a User-Agent.
	req.Header.Set("User-Agent", "invest-assist-market-feed/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("ticker request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return 0, time.Time{}, fmt.Errorf("ticker http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var t coinbaseTicker
	if err := json.Unmarshal(body, &t); err != nil {
		return 0, time.Time{}, fmt.Errorf("decode ticker: %w", err)
	}

	price, err := strconv.ParseFloat(t.Price, 64)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("bad price %q: %w", t.Price, err)
	}
	if price <= 0 {
		return 0, time.Time{}, fmt.Errorf("non-positive price %q for %s", t.Price, product)
	}

	ts := time.Now().UTC()
	if t.Time != "" {
		if parsed, err := time.Parse(time.RFC3339, t.Time); err == nil {
			ts = parsed.UTC()
		}
	}
	return price, ts, nil
}

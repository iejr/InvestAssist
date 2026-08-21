package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// KrakenREST is a thin client over Kraken's public REST API. It currently backs
// the stablecoin->USD bridge (Ticker); it is the intended home for XMR OHLC
// polling later, so it lives in provider/ alongside the Binance clients.
type KrakenREST struct {
	base   string
	client *http.Client
}

// NewKrakenREST constructs a client against the given base URL
// (e.g. "https://api.kraken.com").
func NewKrakenREST(base string) *KrakenREST {
	return &KrakenREST{
		base:   strings.TrimRight(base, "/"),
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// krakenTickerResp mirrors GET /0/public/Ticker. result maps a pair name (which
// Kraken may rename, e.g. USDTUSD -> USDTZUSD) to its ticker fields; we read the
// single entry by iteration rather than assuming the key.
type krakenTickerResp struct {
	Error  []string                     `json:"error"`
	Result map[string]krakenTickerEntry `json:"result"`
}

// krakenTickerEntry holds the fields we use. "c" is [last trade price, lot vol].
type krakenTickerEntry struct {
	C []string `json:"c"`
}

// krakenPair renders a base/quote as Kraken's query pair (e.g. USDT,USD ->
// "USDTUSD"). Kraken accepts the concatenated altname for these markets. BTC
// would need mapping to XBT; not required for the stablecoin bridge.
func krakenPair(base, quote string) string {
	return strings.ToUpper(base) + strings.ToUpper(quote)
}

// Ticker returns the last-trade price of base/quote (e.g. USDT/USD) from
// Kraken. Kraken's ticker carries no per-pair timestamp, so the observation
// time is now (fine for a daily stablecoin snapshot).
func (k *KrakenREST) Ticker(ctx context.Context, base, quote string) (float64, time.Time, error) {
	pair := krakenPair(base, quote)
	q := url.Values{}
	q.Set("pair", pair)
	reqURL := k.base + "/0/public/Ticker?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, time.Time{}, err
	}
	resp, err := k.client.Do(req)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("ticker request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return 0, time.Time{}, fmt.Errorf("ticker http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out krakenTickerResp
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, time.Time{}, fmt.Errorf("decode ticker: %w", err)
	}
	if len(out.Error) > 0 {
		return 0, time.Time{}, fmt.Errorf("kraken error for %s: %s", pair, strings.Join(out.Error, "; "))
	}
	if len(out.Result) == 0 {
		return 0, time.Time{}, fmt.Errorf("no ticker in response for %s", pair)
	}

	// Exactly one entry expected; take it whatever Kraken named the key.
	var entry krakenTickerEntry
	for _, v := range out.Result {
		entry = v
		break
	}
	if len(entry.C) == 0 {
		return 0, time.Time{}, fmt.Errorf("no last-trade price for %s", pair)
	}

	price, err := strconv.ParseFloat(entry.C[0], 64)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("bad price %q for %s: %w", entry.C[0], pair, err)
	}
	if price <= 0 {
		return 0, time.Time{}, fmt.Errorf("non-positive price %q for %s", entry.C[0], pair)
	}
	return price, time.Now().UTC(), nil
}

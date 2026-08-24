package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"market-feed/internal/model"
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

// krakenOHLCMax is the max candles Kraken returns per OHLC request. The public
// endpoint only retains recent data, so deep historical backfill is not possible
// here — acceptable for daily stablecoins and recent XMR windows.
const krakenOHLCMax = 720

// krakenInterval maps our closed interval set to Kraken's OHLC interval, given
// in minutes. Kraken has no sub-minute OHLC, so 1s/5s are rejected.
func krakenInterval(iv model.Interval) (int, error) {
	switch iv {
	case model.Interval1m:
		return 1, nil
	case model.Interval5m:
		return 5, nil
	case model.Interval1h:
		return 60, nil
	case model.Interval1d:
		return 1440, nil
	case model.Interval1s, model.Interval5s:
		return 0, fmt.Errorf("interval %q is not available as a Kraken OHLC", iv)
	default:
		return 0, fmt.Errorf("unknown interval %q", iv)
	}
}

// FetchOHLC fetches candles for [start, end) for one (base, quote) edge at one
// interval. Kraken's OHLC endpoint returns recent data from `since` (capped at
// krakenOHLCMax rows) with no end parameter, so we fetch once and filter to the
// requested window. Returned candles are tagged with the given base/quote.
func (k *KrakenREST) FetchOHLC(ctx context.Context, base, quote string, iv model.Interval, start, end time.Time) ([]model.PriceCandle, error) {
	minutes, err := krakenInterval(iv)
	if err != nil {
		return nil, err
	}
	pair := krakenPair(base, quote)

	q := url.Values{}
	q.Set("pair", pair)
	q.Set("interval", strconv.Itoa(minutes))
	q.Set("since", strconv.FormatInt(start.UTC().Unix(), 10))
	reqURL := k.base + "/0/public/OHLC?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := k.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ohlc request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ohlc http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// result maps the (possibly renamed) pair key to an array of rows, plus a
	// "last" cursor field we skip. Decode loosely to pick out the rows array.
	var out struct {
		Error  []string                   `json:"error"`
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode ohlc: %w", err)
	}
	if len(out.Error) > 0 {
		return nil, fmt.Errorf("kraken error for %s: %s", pair, strings.Join(out.Error, "; "))
	}

	var rows [][]json.RawMessage
	for key, raw := range out.Result {
		if key == "last" {
			continue
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, fmt.Errorf("decode ohlc rows for %s: %w", key, err)
		}
		break
	}

	startUnix := start.UTC().Unix()
	endUnix := end.UTC().Unix()
	candles := make([]model.PriceCandle, 0, len(rows))
	for _, row := range rows {
		c, ok, err := parseKrakenOHLC(row, base, quote, iv)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		// end is exclusive; since already floors the lower bound, but re-check.
		if sec := c.OpenTime.Unix(); sec < startUnix || sec >= endUnix {
			continue
		}
		candles = append(candles, c)
	}

	if len(rows) >= krakenOHLCMax {
		log.Printf("kraken FetchOHLC %s %s: hit %d-row cap; older data in range may be missing", pair, iv, krakenOHLCMax)
	}
	return candles, nil
}

// parseKrakenOHLC converts one Kraken OHLC row into a PriceCandle. A row is a
// positional array [time(sec), open, high, low, close, vwap, volume, count],
// where OHLCV are string-encoded and time/count are numbers.
func parseKrakenOHLC(row []json.RawMessage, base, quote string, iv model.Interval) (model.PriceCandle, bool, error) {
	if len(row) < 7 {
		return model.PriceCandle{}, false, fmt.Errorf("malformed kraken ohlc row (len %d)", len(row))
	}
	var sec int64
	if err := json.Unmarshal(row[0], &sec); err != nil {
		return model.PriceCandle{}, false, fmt.Errorf("bad ohlc time: %w", err)
	}
	open, err1 := krakenNum(row[1])
	high, err2 := krakenNum(row[2])
	low, err3 := krakenNum(row[3])
	cls, err4 := krakenNum(row[4])
	vol, err5 := krakenNum(row[6])
	if err := firstErr(err1, err2, err3, err4, err5); err != nil {
		return model.PriceCandle{}, false, err
	}
	return model.PriceCandle{
		Base:     base,
		Quote:    quote,
		Interval: iv,
		OpenTime: time.Unix(sec, 0).UTC(),
		Open:     open,
		High:     high,
		Low:      low,
		Close:    cls,
		Volume:   vol,
		Source:   model.SourceKraken,
	}, true, nil
}

// krakenNum parses a Kraken string-encoded number.
func krakenNum(raw json.RawMessage) (float64, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, fmt.Errorf("expected string-encoded number, got %s", raw)
	}
	return strconv.ParseFloat(s, 64)
}

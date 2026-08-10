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

	"market-feed/internal/model"
	"market-feed/internal/normalize"
)

// klinesLimit is the max candles Binance returns per klines request.
const klinesLimit = 1000

// BinanceREST fetches historical klines (candlesticks) over REST for backfill.
type BinanceREST struct {
	base   string
	client *http.Client
}

// NewBinanceREST constructs a REST client against the given base URL
// (e.g. "https://api.binance.com" or "https://data-api.binance.vision").
func NewBinanceREST(base string) *BinanceREST {
	return &BinanceREST{
		base:   strings.TrimRight(base, "/"),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// binanceInterval maps our closed interval set to Binance's kline interval
// strings. Binance has no 5s kline, so it is reported unsupported.
func binanceInterval(iv model.Interval) (string, error) {
	switch iv {
	case model.Interval1s:
		return "1s", nil
	case model.Interval1m:
		return "1m", nil
	case model.Interval5m:
		return "5m", nil
	case model.Interval1h:
		return "1h", nil
	case model.Interval1d:
		return "1d", nil
	case model.Interval5s:
		return "", fmt.Errorf("interval %q is not available as a Binance kline", iv)
	default:
		return "", fmt.Errorf("unknown interval %q", iv)
	}
}

// Klines fetches candles for [start, end) for one symbol at one interval,
// paginating until the range is covered. The symbol is Binance notation
// (e.g. "BTCUSDT"); returned candles are normalized to base/quote.
//
// end is treated as exclusive and callers should pass the last *closed*
// interval boundary so an in-progress candle is never fetched.
func (r *BinanceREST) Klines(ctx context.Context, symbol string, iv model.Interval, start, end time.Time) ([]model.PriceCandle, error) {
	bIv, err := binanceInterval(iv)
	if err != nil {
		return nil, err
	}
	base, quote, ok := normalize.Symbol(symbol)
	if !ok {
		return nil, fmt.Errorf("unrecognized symbol %q", symbol)
	}

	var out []model.PriceCandle
	cursor := start.UnixMilli()
	endMs := end.UnixMilli()

	for cursor < endMs {
		rows, err := r.fetchPage(ctx, symbol, bIv, cursor, endMs)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break // no more data in range (e.g. illiquid gap or reached now)
		}

		for _, row := range rows {
			c, ok, err := parseKline(row, base, quote, iv)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			// Guard the exclusive upper bound: Binance may return a candle whose
			// open == endMs when start==end edge cases occur.
			if c.OpenTime.UnixMilli() >= endMs {
				continue
			}
			out = append(out, c)
		}

		// Advance past the last returned open time. openTime is row[0] (ms).
		lastOpen, _ := rows[len(rows)-1][0].(float64)
		next := int64(lastOpen) + 1
		if next <= cursor {
			break // no forward progress; avoid an infinite loop
		}
		cursor = next

		if len(rows) < klinesLimit {
			break // last page
		}
	}

	return out, nil
}

func (r *BinanceREST) fetchPage(ctx context.Context, symbol, interval string, startMs, endMs int64) ([][]any, error) {
	q := url.Values{}
	q.Set("symbol", symbol)
	q.Set("interval", interval)
	q.Set("startTime", strconv.FormatInt(startMs, 10))
	q.Set("endTime", strconv.FormatInt(endMs, 10))
	q.Set("limit", strconv.Itoa(klinesLimit))
	reqURL := r.base + "/api/v3/klines?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("klines request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("klines http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rows [][]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode klines: %w", err)
	}
	return rows, nil
}

// parseKline converts one Binance kline row into a PriceCandle. A kline row is
// a positional array: [openTime, open, high, low, close, volume, closeTime, ...].
func parseKline(row []any, base, quote string, iv model.Interval) (model.PriceCandle, bool, error) {
	if len(row) < 6 {
		return model.PriceCandle{}, false, fmt.Errorf("malformed kline row: %v", row)
	}
	openMs, ok := row[0].(float64)
	if !ok {
		return model.PriceCandle{}, false, fmt.Errorf("bad open time in row: %v", row[0])
	}

	open, err1 := parseFloatField(row[1])
	high, err2 := parseFloatField(row[2])
	low, err3 := parseFloatField(row[3])
	cls, err4 := parseFloatField(row[4])
	vol, err5 := parseFloatField(row[5])
	if err := firstErr(err1, err2, err3, err4, err5); err != nil {
		return model.PriceCandle{}, false, err
	}

	return model.PriceCandle{
		Base:     base,
		Quote:    quote,
		Interval: iv,
		OpenTime: time.UnixMilli(int64(openMs)).UTC(),
		Open:     open,
		High:     high,
		Low:      low,
		Close:    cls,
		Volume:   vol,
		Source:   model.SourceBinance,
	}, true, nil
}

func parseFloatField(v any) (float64, error) {
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("expected string-encoded number, got %T", v)
	}
	return strconv.ParseFloat(s, 64)
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

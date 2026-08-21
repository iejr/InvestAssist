package bridge

import (
	"context"
	"errors"
	"testing"
	"time"

	"market-feed/internal/model"
	"market-feed/internal/provider"
)

// fakeSource returns a fixed price/timestamp (or error) instead of hitting an
// exchange, keeping the poller test deterministic.
type fakeSource struct {
	price float64
	ts    time.Time
	err   error
	calls int
}

func (f *fakeSource) Ticker(_ context.Context, _, _ string) (float64, time.Time, error) {
	f.calls++
	return f.price, f.ts, f.err
}

// fakeLatest captures the last upserted event.
type fakeLatest struct {
	last provider.PriceEvent
	n    int
}

func (l *fakeLatest) Upsert(ev provider.PriceEvent) error {
	l.last = ev
	l.n++
	return nil
}

// fakeCandles captures upserted candles.
type fakeCandles struct {
	rows []model.PriceCandle
}

func (c *fakeCandles) UpsertBatch(candles []model.PriceCandle) error {
	c.rows = append(c.rows, candles...)
	return nil
}

// TestPollOne_WritesLatestAndDailyCandle verifies a single observation lands as
// a latest_prices upsert and a 1d candle whose open_time is the observation's
// UTC day and whose OHLC all equal the spot (a stablecoin barely moves).
func TestPollOne_WritesLatestAndDailyCandle(t *testing.T) {
	// 14:37 UTC should truncate to 00:00 UTC of the same day.
	obs := time.Date(2026, 8, 21, 14, 37, 5, 0, time.UTC)
	src := &fakeSource{price: 0.9998, ts: obs}
	latest := &fakeLatest{}
	candles := &fakeCandles{}
	p := New(src, latest, candles, nil, time.Hour)

	if err := p.pollOne(context.Background(), Edge{Base: "USDT", Quote: "USD"}); err != nil {
		t.Fatalf("pollOne: %v", err)
	}

	if latest.n != 1 {
		t.Fatalf("latest upserts = %d, want 1", latest.n)
	}
	ev := latest.last
	if ev.Base != "USDT" || ev.Quote != "USD" || ev.Price != 0.9998 || ev.Source != model.SourceKraken {
		t.Errorf("latest event = %+v, want USDT/USD 0.9998 kraken", ev)
	}

	if len(candles.rows) != 1 {
		t.Fatalf("candles = %d, want 1", len(candles.rows))
	}
	c := candles.rows[0]
	wantDay := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	if !c.OpenTime.Equal(wantDay) {
		t.Errorf("open_time = %v, want %v (day boundary)", c.OpenTime, wantDay)
	}
	if c.Interval != model.Interval1d {
		t.Errorf("interval = %q, want 1d", c.Interval)
	}
	if c.Open != 0.9998 || c.High != 0.9998 || c.Low != 0.9998 || c.Close != 0.9998 {
		t.Errorf("OHLC = %v/%v/%v/%v, want all 0.9998", c.Open, c.High, c.Low, c.Close)
	}
	if c.Source != model.SourceKraken {
		t.Errorf("source = %q, want kraken", c.Source)
	}
}

// TestPollOne_SourceErrorSkipsWrites ensures a fetch failure does not write a
// partial/zero row to either table.
func TestPollOne_SourceErrorSkipsWrites(t *testing.T) {
	src := &fakeSource{err: errors.New("boom")}
	latest := &fakeLatest{}
	candles := &fakeCandles{}
	p := New(src, latest, candles, nil, time.Hour)

	if err := p.pollOne(context.Background(), Edge{Base: "USDC", Quote: "USD"}); err == nil {
		t.Fatal("expected error, got nil")
	}
	if latest.n != 0 || len(candles.rows) != 0 {
		t.Errorf("wrote despite fetch error: latest=%d candles=%d", latest.n, len(candles.rows))
	}
}

// TestParseEdges covers well-formed, whitespaced, and malformed specs.
func TestParseEdges(t *testing.T) {
	got := ParseEdges([]string{" usdt:usd ", "USDC:USD", "garbage", "onlybase:", ":onlyquote", ""})
	want := []Edge{{Base: "USDT", Quote: "USD"}, {Base: "USDC", Quote: "USD"}}
	if len(got) != len(want) {
		t.Fatalf("got %d edges %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("edge[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

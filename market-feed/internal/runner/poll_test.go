package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"market-feed/internal/model"
)

type fakeTicker struct {
	price float64
	ts    time.Time
	err   error
	calls int
}

func (f *fakeTicker) Ticker(ctx context.Context, base, quote string) (float64, time.Time, error) {
	f.calls++
	return f.price, f.ts, f.err
}

func TestPollFetch_BuildsEvent(t *testing.T) {
	ts := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	src := &fakeTicker{price: 0.9997, ts: ts}
	r := NewPollRunner(src, nil, nil, "USDT", "USD", model.SourceKraken, time.Hour)

	ev, ok := r.fetch(context.Background())
	if !ok {
		t.Fatal("fetch should succeed")
	}
	if ev.Base != "USDT" || ev.Quote != "USD" || ev.Price != 0.9997 {
		t.Errorf("unexpected event: %+v", ev)
	}
	if ev.Source != model.SourceKraken || !ev.Timestamp.Equal(ts) {
		t.Errorf("event source/ts wrong: %+v", ev)
	}
}

func TestPollFetch_SourceErrorSkips(t *testing.T) {
	src := &fakeTicker{err: errors.New("boom")}
	r := NewPollRunner(src, nil, nil, "USDT", "USD", model.SourceKraken, time.Hour)

	if _, ok := r.fetch(context.Background()); ok {
		t.Fatal("fetch should report failure on source error")
	}
}

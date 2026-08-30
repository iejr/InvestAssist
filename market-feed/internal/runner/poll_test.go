package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"market-feed/internal/model"
	"market-feed/internal/provider"
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

type fakeLatest struct {
	events []provider.PriceEvent
}

func (f *fakeLatest) Upsert(ev provider.PriceEvent) error {
	f.events = append(f.events, ev)
	return nil
}

func TestPollObserve_WritesLatest(t *testing.T) {
	ts := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	src := &fakeTicker{price: 0.9997, ts: ts}
	latest := &fakeLatest{}
	r := NewPollRunner(src, latest, "USDT", "USD", model.SourceKraken, time.Hour)

	if err := r.observe(context.Background()); err != nil {
		t.Fatalf("observe: %v", err)
	}
	if len(latest.events) != 1 {
		t.Fatalf("got %d latest writes, want 1", len(latest.events))
	}
	ev := latest.events[0]
	if ev.Base != "USDT" || ev.Quote != "USD" || ev.Price != 0.9997 {
		t.Errorf("unexpected event: %+v", ev)
	}
	if ev.Source != model.SourceKraken || !ev.Timestamp.Equal(ts) {
		t.Errorf("event source/ts wrong: %+v", ev)
	}
}

func TestPollObserve_SourceErrorSkipsWrite(t *testing.T) {
	src := &fakeTicker{err: errors.New("boom")}
	latest := &fakeLatest{}
	r := NewPollRunner(src, latest, "USDT", "USD", model.SourceKraken, time.Hour)

	if err := r.observe(context.Background()); err == nil {
		t.Fatal("expected error from failing source")
	}
	if len(latest.events) != 0 {
		t.Errorf("no latest write expected on source error, got %d", len(latest.events))
	}
}

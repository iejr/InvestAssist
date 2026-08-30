package jobs

import (
	"os"
	"path/filepath"
	"testing"

	"market-feed/internal/model"
)

func TestLoad_ParsesAllKinds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.yaml")
	yaml := `
jobs:
  - kind: stream
    provider: binance
    symbols: [BTCUSDT, ETHUSDT]
  - kind: poll
    provider: kraken
    symbol: USDTUSD
    every: 24h
  - kind: backfill
    provider: kraken
    symbol: USDCUSD
    interval: 1d
    lookback: 720h
    override: false
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(f.Jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(f.Jobs))
	}

	if got := f.Filter(KindStream); len(got) != 1 || len(got[0].Symbols) != 2 {
		t.Errorf("stream filter = %+v", got)
	}

	poll := f.Filter(KindPoll)[0]
	if d, err := poll.EveryDuration(); err != nil || d.Hours() != 24 {
		t.Errorf("poll every = %v, %v", d, err)
	}

	bfl := f.Filter(KindBackfill)[0]
	if iv, err := bfl.CandleInterval(); err != nil || iv != model.Interval1d {
		t.Errorf("backfill interval = %v, %v", iv, err)
	}
	if bfl.OverrideOrDefault() { // explicitly false in the YAML
		t.Errorf("override should be false")
	}
}

func TestOverrideDefaultsTrue(t *testing.T) {
	s := Spec{Kind: KindBackfill} // Override unset
	if !s.OverrideOrDefault() {
		t.Errorf("unset override should default to true")
	}
}

func TestValidate_RejectsBadJobs(t *testing.T) {
	cases := map[string]Spec{
		"stream without symbols": {Kind: KindStream, Provider: "binance"},
		"poll without symbol":    {Kind: KindPoll, Provider: "kraken", Every: "24h"},
		"poll bad every":         {Kind: KindPoll, Provider: "kraken", Symbol: "USDTUSD", Every: "soon"},
		"backfill bad interval":  {Kind: KindBackfill, Provider: "kraken", Symbol: "USDTUSD", Interval: "3d", Lookback: "1h"},
		"backfill bad lookback":  {Kind: KindBackfill, Provider: "kraken", Symbol: "USDTUSD", Interval: "1d", Lookback: "-1h"},
		"missing provider":       {Kind: KindStream, Symbols: []string{"BTCUSDT"}},
		"unknown kind":           {Kind: "frobnicate", Provider: "binance"},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			f := File{Jobs: []Spec{spec}}
			if err := f.Validate(); err == nil {
				t.Errorf("expected validation error for %s", name)
			}
		})
	}
}

func TestValidate_EmptyIsError(t *testing.T) {
	if err := (File{}).Validate(); err == nil {
		t.Errorf("empty job list should be invalid")
	}
}

func TestDefault_IsValid(t *testing.T) {
	f := Default([]string{"BTCUSDT"})
	if err := f.Validate(); err != nil {
		t.Fatalf("default jobs invalid: %v", err)
	}
	if len(f.Filter(KindStream)) != 1 {
		t.Errorf("want one stream job")
	}
	if len(f.Filter(KindPoll)) != 2 || len(f.Filter(KindBackfill)) != 2 {
		t.Errorf("want two poll and two backfill jobs, got %d/%d",
			len(f.Filter(KindPoll)), len(f.Filter(KindBackfill)))
	}
}

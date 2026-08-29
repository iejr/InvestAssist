// Package jobs is the L4 application layer's declarative view of what the
// service should do: a list of heterogeneous jobs (stream / poll / backfill),
// each naming a provider and a symbol (or symbols). It knows nothing about how
// a job runs — that is the runner package's concern — only what was requested.
//
// Jobs are loaded from a YAML file so a growing, heterogeneous list stays
// readable and commentable. Infra config (DB URLs, provider base URLs) remains
// in the environment; only the job list lives here.
package jobs

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"market-feed/internal/model"
)

// Kind is the job type. Each kind maps to a runner and a required provider
// capability (stream→Streamer, poll→TickerFetcher, backfill→OHLCFetcher).
type Kind string

const (
	KindStream   Kind = "stream"
	KindPoll     Kind = "poll"
	KindBackfill Kind = "backfill"
)

// Spec is one job. Fields are shared across kinds; Validate enforces which are
// required for each kind. A stream job lists many native symbols (one WS
// connection, many streams); poll/backfill act on a single symbol.
//
// Note the deliberate split: poll and backfill are separate kinds even for the
// same edge. Poll writes the latest spot to latest_prices on its own cadence;
// backfill writes real OHLC candles over a window. They share lower-layer driver
// code but remain distinct jobs so their schedules can diverge freely.
type Spec struct {
	Kind     Kind     `yaml:"kind"`
	Provider string   `yaml:"provider"`           // "binance" | "kraken"
	Symbols  []string `yaml:"symbols,omitempty"`  // stream: native symbols, e.g. [BTCUSDT]
	Symbol   string   `yaml:"symbol,omitempty"`   // poll/backfill: one native symbol
	Every    string   `yaml:"every,omitempty"`    // poll: cadence, a Go duration (e.g. "24h")
	Interval string   `yaml:"interval,omitempty"` // backfill: candle interval (1m,5m,1h,1d)
	Lookback string   `yaml:"lookback,omitempty"` // backfill: window length back from now (e.g. "720h")
	Override *bool    `yaml:"override,omitempty"` // backfill: overwrite existing rows (default true)
}

// File is the top-level YAML document.
type File struct {
	Jobs []Spec `yaml:"jobs"`
}

// Load reads and validates a jobs file. A missing path is reported as an error;
// callers that want a built-in fallback should check os.IsNotExist and call
// Default instead.
func Load(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return File{}, fmt.Errorf("parse jobs file %s: %w", path, err)
	}
	if err := f.Validate(); err != nil {
		return File{}, fmt.Errorf("invalid jobs file %s: %w", path, err)
	}
	return f, nil
}

// Default is the built-in job list used when no jobs file is present, so local
// development works with zero config: stream the given Binance symbols and keep
// the stablecoin bridge fresh (daily poll for latest, daily 1d OHLC backfill).
func Default(streamSymbols []string) File {
	override := true
	f := File{Jobs: []Spec{
		{Kind: KindStream, Provider: "binance", Symbols: streamSymbols},
	}}
	for _, sym := range []string{"USDTUSD", "USDCUSD"} {
		f.Jobs = append(f.Jobs,
			Spec{Kind: KindPoll, Provider: "kraken", Symbol: sym, Every: "24h"},
			Spec{Kind: KindBackfill, Provider: "kraken", Symbol: sym, Interval: "1d", Lookback: "720h", Override: &override},
		)
	}
	return f
}

// Validate checks per-kind required fields across all jobs, returning the first
// problem found. It catches config mistakes at startup rather than at runtime.
func (f File) Validate() error {
	if len(f.Jobs) == 0 {
		return fmt.Errorf("no jobs configured")
	}
	for i, j := range f.Jobs {
		if err := j.validate(); err != nil {
			return fmt.Errorf("job[%d] (%s): %w", i, j.Kind, err)
		}
	}
	return nil
}

func (s Spec) validate() error {
	if strings.TrimSpace(s.Provider) == "" {
		return fmt.Errorf("provider is required")
	}
	switch s.Kind {
	case KindStream:
		if len(s.Symbols) == 0 {
			return fmt.Errorf("stream requires at least one symbol in `symbols`")
		}
	case KindPoll:
		if strings.TrimSpace(s.Symbol) == "" {
			return fmt.Errorf("poll requires `symbol`")
		}
		if _, err := s.EveryDuration(); err != nil {
			return err
		}
	case KindBackfill:
		if strings.TrimSpace(s.Symbol) == "" {
			return fmt.Errorf("backfill requires `symbol`")
		}
		if _, err := s.CandleInterval(); err != nil {
			return err
		}
		if _, err := s.LookbackDuration(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown kind %q (want stream|poll|backfill)", s.Kind)
	}
	return nil
}

// EveryDuration parses a poll job's cadence.
func (s Spec) EveryDuration() (time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(s.Every))
	if err != nil {
		return 0, fmt.Errorf("bad `every` %q: %w", s.Every, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("`every` must be positive, got %q", s.Every)
	}
	return d, nil
}

// CandleInterval parses a backfill job's candle interval into the model set.
func (s Spec) CandleInterval() (model.Interval, error) {
	switch model.Interval(strings.TrimSpace(s.Interval)) {
	case model.Interval1m:
		return model.Interval1m, nil
	case model.Interval5m:
		return model.Interval5m, nil
	case model.Interval1h:
		return model.Interval1h, nil
	case model.Interval1d:
		return model.Interval1d, nil
	default:
		return "", fmt.Errorf("bad `interval` %q (want 1m|5m|1h|1d)", s.Interval)
	}
}

// LookbackDuration parses a backfill job's window length.
func (s Spec) LookbackDuration() (time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(s.Lookback))
	if err != nil {
		return 0, fmt.Errorf("bad `lookback` %q: %w", s.Lookback, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("`lookback` must be positive, got %q", s.Lookback)
	}
	return d, nil
}

// OverrideOrDefault reports whether backfill should overwrite existing rows,
// defaulting to true (authoritative candles win) when unset.
func (s Spec) OverrideOrDefault() bool {
	if s.Override == nil {
		return true
	}
	return *s.Override
}

// Filter returns the jobs of the given kind, preserving order.
func (f File) Filter(kind Kind) []Spec {
	var out []Spec
	for _, j := range f.Jobs {
		if j.Kind == kind {
			out = append(out, j)
		}
	}
	return out
}

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"market-feed/internal/backfill"
	"market-feed/internal/config"
	"market-feed/internal/db"
	"market-feed/internal/jobs"
	"market-feed/internal/model"
	"market-feed/internal/normalize"
	"market-feed/internal/repository"
	"market-feed/internal/runner"
)

// backfillFlags collects the -backfill CLI options. They apply uniformly to
// both backfill modes — a single ad-hoc -symbol run, and running the backfill
// jobs from the jobs file (the path an external cron takes for the daily
// refresh). Any flag explicitly passed overrides the corresponding YAML value;
// `set` records which flags were given so an unset flag's default never
// clobbers a job's configured value (see resolveBackfill).
type backfillFlags struct {
	enabled  bool
	provider string
	symbol   string
	interval string
	from     string
	to       string
	lookback time.Duration
	override bool
	set      map[string]bool // names of flags explicitly passed on the CLI
}

func main() {
	var bf backfillFlags
	flag.BoolVar(&bf.enabled, "backfill", false, "run backfill then exit (jobs file, or a single -symbol)")
	flag.StringVar(&bf.provider, "provider", "binance", "backfill: provider (ad-hoc default; overrides a job's provider when set)")
	flag.StringVar(&bf.symbol, "symbol", "", "backfill: ad-hoc native symbol (e.g. BTCUSDT); omit to run backfill jobs from the jobs file")
	flag.StringVar(&bf.interval, "interval", "1m", "backfill: candle interval (1s,5s,1m,5m,1h,1d); overrides a job's interval when set")
	flag.StringVar(&bf.from, "from", "", "backfill: start time, RFC3339 or YYYY-MM-DD (UTC); overrides the window in both modes")
	flag.StringVar(&bf.to, "to", "", "backfill: end time, RFC3339 or YYYY-MM-DD (UTC); default now; overrides the window in both modes")
	flag.DurationVar(&bf.lookback, "lookback", 0, "backfill: window length back from -to/now (e.g. 720h); overrides a job's lookback when set")
	flag.BoolVar(&bf.override, "override", true, "backfill: overwrite existing candles (false = fill gaps only); overrides a job's override when set")
	flag.Parse()

	// Record which flags were explicitly given so precedence can distinguish an
	// intentional value from a flag's zero-value default (e.g. -provider defaults
	// to "binance", but in jobs mode a Kraken job must keep its own provider).
	bf.set = map[string]bool{}
	flag.Visit(func(f *flag.Flag) { bf.set[f.Name] = true })

	cfg := config.Load()

	conns, err := db.Open(cfg.DatabaseURL, cfg.HistoryDatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	if bf.enabled {
		runBackfill(cfg, conns, bf)
		return
	}
	runServe(cfg, conns)
}

// loadJobs reads the configured jobs file, or falls back to the built-in default
// (stream MF_SYMBOLS + daily stablecoin bridge) when no file is configured.
func loadJobs(cfg config.Config) jobs.File {
	if cfg.JobsFile == "" {
		return jobs.Default(cfg.Symbols)
	}
	jf, err := jobs.Load(cfg.JobsFile)
	if err != nil {
		log.Fatalf("jobs: %v", err)
	}
	return jf
}

// runServe is the default mode: it starts every long-running job (stream + poll)
// from the jobs file and blocks until interrupted. Backfill is not run here — it
// is one-off, driven externally (cron -> -backfill).
func runServe(cfg config.Config, conns *db.Conns) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	jf := loadJobs(cfg)
	provs := runner.NewProviders(cfg)
	latestRepo := repository.NewLatestRepo(conns.Primary)
	candleRepo := repository.NewCandleRepo(conns.History)

	var wg sync.WaitGroup
	started := 0

	// Stream jobs: one provider connection each, fanned out to latest + sampler.
	for _, spec := range jf.Filter(jobs.KindStream) {
		s, err := provs.Streamer(spec.Provider)
		if err != nil {
			log.Fatalf("job: stream %v: %v", spec.Symbols, err)
		}
		sr := runner.NewStreamRunner(s, spec.Symbols, latestRepo, candleRepo, cfg.SampleIntervals, cfg.LatestCoalesce)
		wg.Add(1)
		started++
		go func() {
			defer wg.Done()
			if err := sr.Run(ctx); err != nil {
				log.Printf("stream: %v", err)
			}
		}()
	}

	// Poll jobs: self-clocked spot observation -> latest_prices.
	for _, spec := range jf.Filter(jobs.KindPoll) {
		base, quote, ok := normalize.Symbol(spec.Symbol)
		if !ok {
			log.Fatalf("job: poll %q: unrecognized symbol", spec.Symbol)
		}
		tf, err := provs.TickerFetcher(spec.Provider)
		if err != nil {
			log.Fatalf("job: poll %s: %v", spec.Symbol, err)
		}
		every, err := spec.EveryDuration() // already validated on load
		if err != nil {
			log.Fatalf("job: poll %s: %v", spec.Symbol, err)
		}
		pr := runner.NewPollRunner(tf, latestRepo, candleRepo, base, quote, runner.SourceFor(spec.Provider), every)
		wg.Add(1)
		started++
		go func() {
			defer wg.Done()
			pr.Run(ctx)
		}()
	}

	if started == 0 {
		log.Println("market-feed: no long-running jobs configured; nothing to serve")
		return
	}
	log.Printf("market-feed: serving %d job(s)", started)

	<-ctx.Done()
	wg.Wait()
	log.Println("market-feed: shutdown complete")
}

// runBackfill runs backfill then exits. Both modes share one path: seed a list
// of job specs (a single synthetic spec from CLI flags when -symbol is given,
// otherwise the backfill jobs from the jobs file), resolve each into a concrete
// window+params, and run them through the same loop.
func runBackfill(cfg config.Config, conns *db.Conns, bf backfillFlags) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	provs := runner.NewProviders(cfg)
	candleRepo := repository.NewCandleRepo(conns.History)
	now := time.Now().UTC()

	specs := seedBackfillSpecs(cfg, bf)
	if len(specs) == 0 {
		log.Println("backfill: no backfill jobs configured")
		return
	}

	ran := 0
	for _, spec := range specs {
		task, err := resolveBackfill(spec, bf, now)
		if err != nil {
			log.Printf("backfill: skip %q: %v", spec.Symbol, err)
			continue
		}
		fetcher, err := provs.OHLCFetcher(task.provider)
		if err != nil {
			log.Printf("backfill: skip %s/%s: %v", task.base, task.quote, err)
			continue
		}
		rr := runner.NewBackfillRunner(backfill.New(fetcher, candleRepo), task.base, task.quote, task.iv, task.start, task.end, task.override)
		if err := rr.Run(ctx); err != nil {
			log.Printf("backfill %s/%s: %v", task.base, task.quote, err)
			continue
		}
		ran++
	}
	log.Printf("backfill: done (%d/%d)", ran, len(specs))
}

// seedBackfillSpecs produces the specs to run: one synthetic spec built from the
// CLI flags when -symbol is set (ad-hoc mode), otherwise the backfill jobs from
// the jobs file. The synthetic spec carries the raw flag values; resolveBackfill
// applies the same overlay to it as to a YAML job, so the two modes converge.
func seedBackfillSpecs(cfg config.Config, bf backfillFlags) []jobs.Spec {
	if bf.symbol != "" {
		override := bf.override
		return []jobs.Spec{{
			Kind:     jobs.KindBackfill,
			Provider: bf.provider,
			Symbol:   bf.symbol,
			Interval: bf.interval,
			Override: &override,
			// Lookback intentionally empty: an ad-hoc window comes from -from/-to
			// or -lookback, not from YAML.
		}}
	}
	return loadJobs(cfg).Filter(jobs.KindBackfill)
}

// backfillTask is a fully-resolved unit of backfill work.
type backfillTask struct {
	provider    string
	base, quote string
	iv          model.Interval
	start, end  time.Time
	override    bool
}

// resolveBackfill overlays the CLI flags onto a job spec to produce a concrete
// task. Precedence per field is: explicit CLI flag > spec (YAML) value >
// default. The window is derived last: end = -to | now; start = -from |
// (end - lookback), where lookback = -lookback | spec.Lookback.
func resolveBackfill(spec jobs.Spec, bf backfillFlags, now time.Time) (backfillTask, error) {
	base, quote, ok := normalize.Symbol(spec.Symbol)
	if !ok {
		return backfillTask{}, fmt.Errorf("unrecognized symbol")
	}

	provider := spec.Provider
	if bf.set["provider"] {
		provider = bf.provider
	}

	intervalStr := spec.Interval
	if bf.set["interval"] {
		intervalStr = bf.interval
	}
	iv, err := parseInterval(intervalStr)
	if err != nil {
		return backfillTask{}, err
	}

	override := spec.OverrideOrDefault()
	if bf.set["override"] {
		override = bf.override
	}

	// end: -to overrides now.
	end := now
	if bf.set["to"] {
		if end, err = parseTime(bf.to); err != nil {
			return backfillTask{}, fmt.Errorf("bad -to: %w", err)
		}
	}

	// start: -from wins outright; otherwise derive from a lookback window
	// (CLI -lookback overrides the job's YAML lookback).
	var start time.Time
	switch {
	case bf.set["from"]:
		if start, err = parseTime(bf.from); err != nil {
			return backfillTask{}, fmt.Errorf("bad -from: %w", err)
		}
	case bf.set["lookback"]:
		start = end.Add(-bf.lookback)
	case strings.TrimSpace(spec.Lookback) != "":
		lookback, err := spec.LookbackDuration()
		if err != nil {
			return backfillTask{}, err
		}
		start = end.Add(-lookback)
	default:
		return backfillTask{}, fmt.Errorf("no window: pass -from, or set -lookback / a job lookback")
	}

	return backfillTask{provider: provider, base: base, quote: quote, iv: iv, start: start, end: end, override: override}, nil
}

// parseInterval validates a backfill interval string against the full stored set
// (allowing sub-minute intervals for ad-hoc runs, unlike a YAML poll/backfill
// job which is restricted to 1m|5m|1h|1d).
func parseInterval(s string) (model.Interval, error) {
	iv := model.Interval(strings.TrimSpace(s))
	if _, err := backfill.IntervalDuration(iv); err != nil {
		return "", fmt.Errorf("bad interval %q: %w", s, err)
	}
	return iv, nil
}

// parseTime accepts RFC3339 or a plain YYYY-MM-DD date (interpreted as UTC).
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

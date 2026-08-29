package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
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
	"market-feed/internal/sampler"
)

// backfillFlags collects the -backfill CLI options for an ad-hoc, one-off run.
// When -symbol is omitted, -backfill instead runs the backfill jobs from the
// jobs file (the path an external cron would take for the daily refresh).
type backfillFlags struct {
	enabled  bool
	provider string
	symbol   string
	interval string
	from     string
	to       string
	override bool
}

func main() {
	var bf backfillFlags
	flag.BoolVar(&bf.enabled, "backfill", false, "run backfill then exit (jobs file, or a single -symbol)")
	flag.StringVar(&bf.provider, "provider", "binance", "backfill: provider for an ad-hoc -symbol run")
	flag.StringVar(&bf.symbol, "symbol", "", "backfill: ad-hoc native symbol (e.g. BTCUSDT); omit to run backfill jobs from the jobs file")
	flag.StringVar(&bf.interval, "interval", "1m", "backfill: candle interval (1m,5m,1h,1d)")
	flag.StringVar(&bf.from, "from", "", "backfill: start time, RFC3339 or YYYY-MM-DD (UTC)")
	flag.StringVar(&bf.to, "to", "", "backfill: end time, RFC3339 or YYYY-MM-DD (UTC); default now")
	flag.BoolVar(&bf.override, "override", true, "backfill: overwrite existing candles (false = fill gaps only)")
	flag.Parse()

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
		smp := sampler.New(cfg.SampleInterval, candleRepo)
		sr := runner.NewStreamRunner(s, spec.Symbols, latestRepo, smp, cfg.SampleInterval)
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
		pr := runner.NewPollRunner(tf, latestRepo, base, quote, runner.SourceFor(spec.Provider), every)
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

// runBackfill runs backfill then exits. With -symbol it does a single ad-hoc
// range; without it, it runs every backfill job from the jobs file over each
// job's rolling lookback window.
func runBackfill(cfg config.Config, conns *db.Conns, bf backfillFlags) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	provs := runner.NewProviders(cfg)
	candleRepo := repository.NewCandleRepo(conns.History)
	now := time.Now().UTC()

	if bf.symbol != "" {
		runAdHocBackfill(ctx, provs, candleRepo, bf, now)
		return
	}

	specs := loadJobs(cfg).Filter(jobs.KindBackfill)
	if len(specs) == 0 {
		log.Println("backfill: no backfill jobs configured")
		return
	}
	for _, spec := range specs {
		base, quote, ok := normalize.Symbol(spec.Symbol)
		if !ok {
			log.Printf("backfill: skip %q: unrecognized symbol", spec.Symbol)
			continue
		}
		fetcher, err := provs.OHLCFetcher(spec.Provider)
		if err != nil {
			log.Printf("backfill: skip %s: %v", spec.Symbol, err)
			continue
		}
		iv, _ := spec.CandleInterval()      // validated on load
		lookback, _ := spec.LookbackDuration() // validated on load
		rr := runner.NewBackfillRunner(backfill.New(fetcher, candleRepo), base, quote, iv, lookback, spec.OverrideOrDefault())
		if err := rr.Run(ctx, now); err != nil {
			log.Printf("backfill %s/%s: %v", base, quote, err)
		}
	}
	log.Println("backfill: done")
}

// runAdHocBackfill handles the single-symbol CLI path.
func runAdHocBackfill(ctx context.Context, provs *runner.Providers, candleRepo *repository.CandleRepo, bf backfillFlags, now time.Time) {
	if bf.from == "" {
		log.Fatal("backfill: -from is required with -symbol")
	}
	base, quote, ok := normalize.Symbol(bf.symbol)
	if !ok {
		log.Fatalf("backfill: unrecognized -symbol %q", bf.symbol)
	}
	start, err := parseTime(bf.from)
	if err != nil {
		log.Fatalf("backfill: bad -from: %v", err)
	}
	end := now
	if bf.to != "" {
		if end, err = parseTime(bf.to); err != nil {
			log.Fatalf("backfill: bad -to: %v", err)
		}
	}
	fetcher, err := provs.OHLCFetcher(bf.provider)
	if err != nil {
		log.Fatalf("backfill: %v", err)
	}
	bfr := backfill.New(fetcher, candleRepo)
	if err := bfr.Run(ctx, base, quote, model.Interval(bf.interval), start, end, bf.override); err != nil {
		log.Fatalf("backfill: %v", err)
	}
	log.Println("backfill: done")
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

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"market-feed/internal/backfill"
	"market-feed/internal/bridge"
	"market-feed/internal/bus"
	"market-feed/internal/config"
	"market-feed/internal/db"
	"market-feed/internal/model"
	"market-feed/internal/provider"
	"market-feed/internal/repository"
	"market-feed/internal/sampler"
)

// backfillFlags collects the -backfill CLI options.
type backfillFlags struct {
	enabled  bool
	symbol   string
	interval string
	from     string
	to       string
	override bool
}

func main() {
	var bf backfillFlags
	flag.BoolVar(&bf.enabled, "backfill", false, "run a one-off historical backfill then exit")
	flag.StringVar(&bf.symbol, "symbol", "", "backfill: Binance symbol (e.g. BTCUSDT)")
	flag.StringVar(&bf.interval, "interval", "1m", "backfill: candle interval (1s,1m,5m,1h,1d)")
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
	runStream(cfg, conns)
}

// runBackfill fetches historical klines for the requested range then returns.
func runBackfill(cfg config.Config, conns *db.Conns, bf backfillFlags) {
	if bf.symbol == "" || bf.from == "" {
		log.Fatal("backfill: -symbol and -from are required")
	}
	start, err := parseTime(bf.from)
	if err != nil {
		log.Fatalf("backfill: bad -from: %v", err)
	}
	end := time.Now().UTC()
	if bf.to != "" {
		if end, err = parseTime(bf.to); err != nil {
			log.Fatalf("backfill: bad -to: %v", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rest := provider.NewBinanceREST(cfg.BinanceRESTBase)
	repo := repository.NewCandleRepo(conns.History)
	bfr := backfill.New(rest, repo)

	if err := bfr.Run(ctx, bf.symbol, model.Interval(bf.interval), start, end, bf.override); err != nil {
		log.Fatalf("backfill: %v", err)
	}
	log.Println("backfill: done")
}

// runStream is the default mode: live Binance WS -> latest_prices + candles.
func runStream(cfg config.Config, conns *db.Conns) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Ingestion: Binance WS -> normalized PriceEvents.
	prov := provider.NewBinanceWS(cfg.BinanceWSBase)
	events, err := prov.Stream(ctx, cfg.Symbols)
	if err != nil {
		log.Fatalf("provider: %v", err)
	}

	// Fan out each event to the latest-price writer and the sampler.
	b := bus.New()
	latestCh := b.Subscribe(1024)
	sampleCh := b.Subscribe(1024)

	latestRepo := repository.NewLatestRepo(conns.Primary)
	candleRepo := repository.NewCandleRepo(conns.History)
	smp := sampler.New(cfg.SampleInterval, candleRepo)

	// Latest-price writer.
	go func() {
		for ev := range latestCh {
			if err := latestRepo.Upsert(ev); err != nil {
				log.Printf("latest: upsert %s/%s: %v", ev.Base, ev.Quote, err)
			}
		}
	}()

	// OHLC sampler.
	go smp.Run(ctx, sampleCh)

	// Stablecoin->USD bridge poller (Coinbase). Self-clocked, independent of the
	// Binance bus/sampler: latest rows to Primary, daily-close candles to History.
	coinbase := bridge.NewCoinbaseREST(cfg.CoinbaseRESTBase)
	bridgePoller := bridge.New(coinbase, latestRepo, candleRepo, bridge.ParseEdges(cfg.Bridges), cfg.BridgeInterval)
	go bridgePoller.Run(ctx)

	log.Printf("market-feed: streaming %v, sampling every %s", cfg.Symbols, cfg.SampleInterval)

	// Pump provider events into the bus until the stream closes (ctx cancelled).
	for ev := range events {
		b.Publish(ev)
	}

	b.Close()
	log.Println("market-feed: shutdown complete")
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

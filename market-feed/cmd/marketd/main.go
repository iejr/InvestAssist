package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"market-feed/internal/bus"
	"market-feed/internal/config"
	"market-feed/internal/db"
	"market-feed/internal/provider"
	"market-feed/internal/repository"
	"market-feed/internal/sampler"
)

func main() {
	cfg := config.Load()

	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

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

	latestRepo := repository.NewLatestRepo(gdb)
	candleRepo := repository.NewCandleRepo(gdb)
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

	log.Printf("market-feed: streaming %v, sampling every %s", cfg.Symbols, cfg.SampleInterval)

	// Pump provider events into the bus until the stream closes (ctx cancelled).
	for ev := range events {
		b.Publish(ev)
	}

	b.Close()
	log.Println("market-feed: shutdown complete")
}

package config

import (
	"os"
	"strings"
	"time"
)

// Config holds all runtime configuration for the market-feed service.
// Everything is sourced from environment variables so the service can share
// the same Postgres instance as server-go without extra wiring.
type Config struct {
	// DatabaseURL is the shared Postgres DSN (same DB as server-go). Holds
	// latest_prices, and price_candles too unless HistoryDatabaseURL is set.
	DatabaseURL string

	// HistoryDatabaseURL, when non-empty, routes append-only history
	// (price_candles) to a separate Postgres instance. Empty means history
	// lives alongside latest_prices in DatabaseURL.
	HistoryDatabaseURL string

	// Symbols are the Binance symbols to stream, in Binance's own notation
	// (e.g. "BTCUSDT"). We start with BTC/USDT only.
	Symbols []string

	// SampleIntervals are the OHLC bucket sizes the stream samplers emit. One
	// sampler runs per interval off the same bus (e.g. 1m and 5m), so a single
	// connection yields multiple candle resolutions.
	SampleIntervals []time.Duration

	// LatestCoalesce bounds how often latest_prices is written per edge: at most
	// one write per window, keeping only the newest value. 0 disables coalescing
	// (write every event). Stream ticks can arrive many times per second; a small
	// window keeps Postgres from being hammered without meaningfully staleness.
	LatestCoalesce time.Duration

	// BinanceWSBase is the Binance combined-stream websocket base URL.
	BinanceWSBase string

	// BinanceRESTBase is the Binance REST base URL, used for klines backfill.
	BinanceRESTBase string

	// KrakenRESTBase is the Kraken REST base URL, used to observe stablecoin->USD
	// bridge rates (USDT/USD, USDC/USD) and, later, XMR OHLC.
	KrakenRESTBase string

	// JobsFile is the path to the YAML job list (L4). When empty or missing, a
	// built-in default is used so local development runs with zero config.
	JobsFile string
}

// Load reads configuration from the environment, applying sensible defaults
// for local development.
func Load() Config {
	return Config{
		DatabaseURL:        envOr("DATABASE_URL", "host=localhost user=postgres password=postgres dbname=invest_assist port=5432 sslmode=disable"),
		HistoryDatabaseURL: strings.TrimSpace(os.Getenv("MF_HISTORY_DATABASE_URL")),
		Symbols:            csvOr("MF_SYMBOLS", []string{"BTCUSDT"}),
		SampleIntervals:    durationsOr("MF_SAMPLE_INTERVALS", []time.Duration{time.Minute}),
		LatestCoalesce:     durationOr("MF_LATEST_COALESCE", time.Second),
		BinanceWSBase:      envOr("MF_BINANCE_WS", "wss://stream.binance.com:9443"),
		BinanceRESTBase:    envOr("MF_BINANCE_REST", "https://api.binance.com"),
		KrakenRESTBase:     envOr("MF_KRAKEN_REST", "https://api.kraken.com"),
		JobsFile:           strings.TrimSpace(os.Getenv("MF_JOBS_FILE")),
	}
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func csvOr(key string, def []string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func durationOr(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

// durationsOr parses a comma-separated list of Go durations, ignoring blanks
// and unparseable entries. If nothing valid remains, the default is returned.
func durationsOr(key string, def []time.Duration) []time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	var out []time.Duration
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if d, err := time.ParseDuration(p); err == nil && d > 0 {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

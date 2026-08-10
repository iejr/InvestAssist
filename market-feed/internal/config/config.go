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

	// SampleInterval is the OHLC bucket size for the historical sampler.
	SampleInterval time.Duration

	// BinanceWSBase is the Binance combined-stream websocket base URL.
	BinanceWSBase string

	// BinanceRESTBase is the Binance REST base URL, used for klines backfill.
	BinanceRESTBase string

	// CoinbaseRESTBase is the Coinbase Exchange REST base URL, used to observe
	// stablecoin->USD bridge rates (e.g. USDT/USD, USDC/USD).
	CoinbaseRESTBase string

	// Bridges are the stablecoin->USD edges to poll from Coinbase, each in
	// "BASE:QUOTE" form (e.g. "USDT:USD"). These never assume a 1:1 peg.
	Bridges []string

	// BridgeInterval is how often the bridge poller samples Coinbase. Stablecoin
	// rates barely move, so a daily cadence is plenty for portfolio valuation.
	BridgeInterval time.Duration
}

// Load reads configuration from the environment, applying sensible defaults
// for local development.
func Load() Config {
	return Config{
		DatabaseURL:        envOr("DATABASE_URL", "host=localhost user=postgres password=postgres dbname=invest_assist port=5432 sslmode=disable"),
		HistoryDatabaseURL: strings.TrimSpace(os.Getenv("MF_HISTORY_DATABASE_URL")),
		Symbols:            csvOr("MF_SYMBOLS", []string{"BTCUSDT"}),
		SampleInterval:     durationOr("MF_SAMPLE_INTERVAL", time.Minute),
		BinanceWSBase:      envOr("MF_BINANCE_WS", "wss://stream.binance.com:9443"),
		BinanceRESTBase:    envOr("MF_BINANCE_REST", "https://api.binance.com"),
		CoinbaseRESTBase:   envOr("MF_COINBASE_REST", "https://api.exchange.coinbase.com"),
		Bridges:            csvOr("MF_BRIDGES", []string{"USDT:USD", "USDC:USD"}),
		BridgeInterval:     durationOr("MF_BRIDGE_INTERVAL", 24*time.Hour),
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

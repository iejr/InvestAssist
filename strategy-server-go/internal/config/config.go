// Package config sources runtime configuration from the environment. Defaults
// match market-feed so this service shares the same Postgres by default.
package config

import (
	"os"
	"strings"
	"time"
)

// Config holds all runtime configuration for the strategy server.
type Config struct {
	// DatabaseURL is the shared Postgres DSN. Holds the strategy tables plus
	// latest_prices, and price_candles too unless HistoryDatabaseURL is set.
	DatabaseURL string

	// HistoryDatabaseURL, when non-empty, is where price_candles live (matches
	// market-feed's MF_HISTORY_DATABASE_URL split). Empty ⇒ candles share
	// DatabaseURL.
	HistoryDatabaseURL string

	// Port is the HTTP listen port.
	Port string

	// MaxStalenessOverride, when > 0, overrides the default per-strategy
	// staleness window (one interval-step). Beyond it, a price is reported
	// missing rather than fabricated.
	MaxStalenessOverride time.Duration
}

// Load reads configuration, applying local-development defaults.
func Load() Config {
	return Config{
		DatabaseURL:          envOr("DATABASE_URL", "host=localhost user=postgres password=postgres dbname=invest_assist port=5432 sslmode=disable"),
		HistoryDatabaseURL:   strings.TrimSpace(os.Getenv("MF_HISTORY_DATABASE_URL")),
		Port:                 envOr("PORT", "3000"),
		MaxStalenessOverride: durationOr("STRATEGY_MAX_STALENESS", 0),
	}
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
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

// Package config sources runtime configuration from the environment. Defaults
// match market-feed so this service shares the same Postgres by default.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for the strategy server.
type Config struct {
	// DatabaseURL is the shared Postgres DSN. Holds the strategy tables plus
	// price_candles.
	DatabaseURL string

	// RedisAddr is the Redis address (host:port) backing latest_prices,
	// shared with market-feed.
	RedisAddr string

	// RedisDB selects the Redis logical DB index.
	RedisDB int

	// RedisPassword authenticates to Redis, if required.
	RedisPassword string

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
		RedisAddr:            envOr("MF_REDIS_ADDR", "localhost:6379"),
		RedisDB:              intOr("MF_REDIS_DB", 0),
		RedisPassword:        os.Getenv("MF_REDIS_PASSWORD"),
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

func intOr(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
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

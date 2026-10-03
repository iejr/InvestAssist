// Package latestcache writes the newest observed value for each (base, quote)
// edge to Redis, replacing the old Postgres latest_prices table.
package latestcache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"market-feed/internal/model"
	"market-feed/internal/provider"

	"github.com/redis/go-redis/v9"
)

// Key is the single Redis HASH holding every edge. Field "{base}/{quote}"
// maps to a JSON-encoded value — one HASH, not per-edge STRING keys, because
// readers (strategy-server-go) always want the whole snapshot (HGETALL),
// never a point lookup.
const Key = "latest_prices"

// value is the JSON shape stored per HASH field.
type value struct {
	Price     float64      `json:"price"`
	Source    model.Source `json:"source"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// Cache upserts the newest observed value for each edge into Redis.
type Cache struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Cache { return &Cache{rdb: rdb} }

// Upsert writes ev as the latest price for its edge, overwriting any existing
// value for the same (base, quote).
func (c *Cache) Upsert(ev provider.PriceEvent) error {
	v := value{Price: ev.Price, Source: ev.Source, UpdatedAt: ev.Timestamp}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("latestcache: marshal %s/%s: %w", ev.Base, ev.Quote, err)
	}
	field := ev.Base + "/" + ev.Quote
	return c.rdb.HSet(context.Background(), Key, field, b).Err()
}

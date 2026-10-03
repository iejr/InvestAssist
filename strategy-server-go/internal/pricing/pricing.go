// Package pricing owns valuation: it reads market-feed's data directly
// (Decision A1/B1) and converts an asset into a quote/display currency at an
// arbitrary time.
//
// Facts are stored in native quote (e.g. BTC/USDT), so conversion is a walk over
// the edges that actually exist in latest_prices. Routes are DISCOVERED from the
// data, not hardcoded: if BTC/USDT and USDT/USD both exist, BTC→USD is a 2-hop
// walk; when new edges appear, new routes light up with no code change.
//
// Prices are never fabricated. A backward as-of lookup that finds nothing at or
// before the requested time — or whose newest observation is staler than the
// caller's window — reports missing, and the caller treats the contribution as
// n/a rather than inventing a number.
package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"strategy-server-go/internal/market"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// latestPricesKey is the single Redis HASH market-feed writes latest_prices
// into. Field "{base}/{quote}" maps to a JSON-encoded market.LatestPrice.
const latestPricesKey = "latest_prices"

// Pricer answers valuation queries against the feed data.
type Pricer struct {
	latest  *redis.Client // holds latest_prices
	candles *gorm.DB      // holds price_candles
}

// New builds a Pricer. latest reads latest_prices from Redis; candles reads
// price_candles from Postgres.
func New(latest *redis.Client, candles *gorm.DB) *Pricer {
	return &Pricer{latest: latest, candles: candles}
}

// hop is one directed step of a route. from/to is the traversal direction;
// invert says whether that runs against the stored edge orientation. invert is
// the only primitive of the three — the stored orientation (what candle lookups
// must query) is derived from it via storedBase/storedQuote.
type hop struct {
	from      string
	to        string
	price     float64   // latest price in the stored orientation
	updatedAt time.Time // latest_prices freshness for this edge
	invert    bool      // true when traversing storedQuote -> storedBase
}

// storedBase / storedQuote recover the orientation the edge is actually stored
// in (latest_prices, price_candles). Candle lookups must query that orientation
// and then apply invert to the retrieved close.
func (h hop) storedBase() string {
	if h.invert {
		return h.to
	}
	return h.from
}

func (h hop) storedQuote() string {
	if h.invert {
		return h.from
	}
	return h.to
}

// rate returns the multiplier for 1 unit of `from` expressed in `to`, using the
// hop's latest price.
func (h hop) rate() float64 {
	if h.invert {
		return 1 / h.price
	}
	return h.price
}

// graph is an in-memory snapshot of the edges present in latest_prices.
type graph struct {
	adj map[string]map[string]hop
}

// snapshot loads every edge from latest_prices (one HGETALL) and builds a
// bidirectional graph.
func (p *Pricer) snapshot() (*graph, error) {
	raw, err := p.latest.HGetAll(context.Background(), latestPricesKey).Result()
	if err != nil {
		return nil, fmt.Errorf("pricing: HGETALL %s: %w", latestPricesKey, err)
	}
	return buildGraph(raw)
}

// buildGraph turns the raw HGETALL result (field "{base}/{quote}" -> JSON
// market.LatestPrice) into a bidirectional graph. Split out from snapshot so
// the graph-construction logic is unit-testable without a Redis connection.
func buildGraph(raw map[string]string) (*graph, error) {
	g := &graph{adj: make(map[string]map[string]hop)}
	for field, val := range raw {
		base, quote, ok := strings.Cut(field, "/")
		if !ok {
			continue // malformed field; skip rather than fail the whole snapshot.
		}
		var lp market.LatestPrice
		if err := json.Unmarshal([]byte(val), &lp); err != nil {
			return nil, fmt.Errorf("pricing: decode %q: %w", field, err)
		}
		if lp.Price <= 0 {
			// Non-positive rate is corrupt / non-invertible (a legitimately tiny
			// price like 0.000008 is still positive and kept). Skip the edge.
			continue
		}
		g.link(base, quote, hop{
			from: base, to: quote, price: lp.Price, updatedAt: lp.UpdatedAt, invert: false,
		})
		g.link(quote, base, hop{
			from: quote, to: base, price: lp.Price, updatedAt: lp.UpdatedAt, invert: true,
		})
	}
	return g, nil
}

// bases returns every distinct edge endpoint in the graph, sorted. Used by
// Pricer.Bases instead of a separate query — every base that has at least one
// edge is a key of adj.
func (g *graph) bases() []string {
	out := make([]string, 0, len(g.adj))
	for b := range g.adj {
		out = append(out, b)
	}
	sort.Strings(out)
	return out
}

func (g *graph) link(from, to string, h hop) {
	if g.adj[from] == nil {
		g.adj[from] = make(map[string]hop)
	}
	// First edge wins if a pair somehow appears twice; latest_prices is unique
	// per (base,quote) so this is just defensive.
	if _, ok := g.adj[from][to]; !ok {
		g.adj[from][to] = h
	}
}

// route returns the shortest hop sequence from base to quote (BFS over the
// undirected edge set). ok is false when the two are not connected.
func (g *graph) route(base, quote string) (path []hop, ok bool) {
	if base == quote {
		return nil, true // identity route: rate 1, no hops.
	}
	if _, seen := g.adj[base]; !seen {
		return nil, false
	}
	prev := map[string]hop{}
	visited := map[string]bool{base: true}
	queue := []string{base}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for next, h := range g.adj[cur] {
			if visited[next] {
				continue
			}
			visited[next] = true
			prev[next] = h
			if next == quote {
				return backtrack(prev, base, quote), true
			}
			queue = append(queue, next)
		}
	}
	return nil, false
}

// backtrack rebuilds the base→quote hop list from the BFS predecessor map.
func backtrack(prev map[string]hop, base, quote string) []hop {
	var rev []hop
	for cur := quote; cur != base; {
		h := prev[cur]
		rev = append(rev, h)
		cur = h.from
	}
	// rev is quote→base; reverse to base→quote.
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// Bases returns the distinct assets usable as a strategy base — every base that
// appears in latest_prices, sorted.
func (p *Pricer) Bases() ([]string, error) {
	g, err := p.snapshot()
	if err != nil {
		return nil, err
	}
	return g.bases(), nil
}

// ReachableQuotes returns every currency reachable from base (direct or via a
// multi-hop route), sorted. These are the quote options at creation and the
// display-currency options at read time. Excludes base itself.
func (p *Pricer) ReachableQuotes(base string) ([]string, error) {
	g, err := p.snapshot()
	if err != nil {
		return nil, err
	}
	if _, ok := g.adj[base]; !ok {
		return []string{}, nil
	}
	visited := map[string]bool{base: true}
	queue := []string{base}
	var out []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for next := range g.adj[cur] {
			if visited[next] {
				continue
			}
			visited[next] = true
			out = append(out, next)
			queue = append(queue, next)
		}
	}
	sort.Strings(out)
	return out, nil
}

// ErrNoRoute means base and quote are not connected by any edge in the feed.
var ErrNoRoute = errors.New("pricing: no route between assets")

// LatestPrice returns the live price of 1 base in quote using latest_prices,
// walking the shortest route. asOf is the freshest edge's timestamp along the
// route (the oldest of the hops — i.e. the route is only as fresh as its
// stalest edge). ok is false when no route exists.
func (p *Pricer) LatestPrice(base, quote string) (price float64, asOf time.Time, ok bool, err error) {
	g, err := p.snapshot()
	if err != nil {
		return 0, time.Time{}, false, err
	}
	path, connected := g.route(base, quote)
	if !connected {
		return 0, time.Time{}, false, nil
	}
	price = 1
	for i, h := range path {
		price *= h.rate()
		if i == 0 || h.updatedAt.Before(asOf) {
			asOf = h.updatedAt
		}
	}
	return price, asOf, true, nil
}

// PriceAt returns the price of 1 base in quote as-of time t, using daily-close
// candles with a backward as-of join per hop. maxStaleness bounds how old the
// newest observation at-or-before t may be; beyond it the hop (and the whole
// route) is reported missing (ok=false). A missing/absent candle for any hop
// makes the whole route missing — prices are never fabricated.
func (p *Pricer) PriceAt(base, quote string, t time.Time, maxStaleness time.Duration) (price float64, ok bool, err error) {
	g, err := p.snapshot()
	if err != nil {
		return 0, false, err
	}
	path, connected := g.route(base, quote)
	if !connected {
		return 0, false, nil
	}
	price = 1
	for _, h := range path {
		rate, hopOK, hErr := p.hopPriceAt(h, t, maxStaleness)
		if hErr != nil {
			return 0, false, hErr
		}
		if !hopOK {
			return 0, false, nil
		}
		price *= rate
	}
	return price, true, nil
}

// hopPriceAt fetches the daily-close as-of price for a single hop.
func (p *Pricer) hopPriceAt(h hop, t time.Time, maxStaleness time.Duration) (rate float64, ok bool, err error) {
	var c market.PriceCandle
	q := p.candles.
		Where("base = ? AND quote = ? AND interval = ? AND open_time <= ?",
			h.storedBase(), h.storedQuote(), market.Interval1d, t).
		Order("open_time desc").
		Limit(1)
	if err := q.First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, false, nil // no observation at-or-before t ⇒ missing.
		}
		return 0, false, err
	}
	if maxStaleness > 0 && t.Sub(c.OpenTime) > maxStaleness {
		return 0, false, nil // newest observation is too old ⇒ missing.
	}
	rate = c.Close
	if h.invert {
		if rate == 0 {
			return 0, false, nil
		}
		rate = 1 / rate
	}
	return rate, true, nil
}

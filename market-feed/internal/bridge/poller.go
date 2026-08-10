package bridge

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"market-feed/internal/model"
	"market-feed/internal/provider"
)

// rateSource observes a stablecoin->USD spot rate. Satisfied by *CoinbaseREST;
// an interface so the poller is testable without a live Coinbase.
type rateSource interface {
	Ticker(ctx context.Context, product string) (float64, time.Time, error)
}

// latestStore upserts the newest value for an edge (satisfied by LatestRepo).
type latestStore interface {
	Upsert(ev provider.PriceEvent) error
}

// candleStore writes daily-close candles (satisfied by CandleRepo.UpsertBatch).
// Override semantics: re-polling the same UTC day overwrites that day's row,
// which is what we want for a near-constant stablecoin rate.
type candleStore interface {
	UpsertBatch(candles []model.PriceCandle) error
}

// Compile-time checks that the real types satisfy the seams.
var (
	_ rateSource = (*CoinbaseREST)(nil)
)

// Edge is one stablecoin->USD bridge to poll, e.g. base=USDT, quote=USD.
type Edge struct {
	Base  string
	Quote string
}

// ParseEdges turns "USDT:USD,USDC:USD" config entries into Edges, skipping and
// logging malformed ones rather than failing the whole service.
func ParseEdges(specs []string) []Edge {
	var out []Edge
	for _, s := range specs {
		parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
		if len(parts) != 2 {
			log.Printf("bridge: ignoring malformed edge %q (want BASE:QUOTE)", s)
			continue
		}
		base := strings.ToUpper(strings.TrimSpace(parts[0]))
		quote := strings.ToUpper(strings.TrimSpace(parts[1]))
		if base == "" || quote == "" {
			log.Printf("bridge: ignoring malformed edge %q (want BASE:QUOTE)", s)
			continue
		}
		out = append(out, Edge{Base: base, Quote: quote})
	}
	return out
}

// product renders an edge as a Coinbase product id, e.g. "USDT-USD".
func (e Edge) product() string { return e.Base + "-" + e.Quote }

// Poller samples stablecoin->USD bridge rates from Coinbase on a slow timer and
// writes them to latest_prices (newest edge) and price_candles (daily close).
// It is a self-clocked layer, deliberately separate from the Binance bus/sampler
// pipeline (see DESIGN.md "Three layers, each self-clocked").
type Poller struct {
	src      rateSource
	latest   latestStore
	candles  candleStore
	edges    []Edge
	interval time.Duration
}

// New constructs a bridge poller.
func New(src rateSource, latest latestStore, candles candleStore, edges []Edge, interval time.Duration) *Poller {
	return &Poller{src: src, latest: latest, candles: candles, edges: edges, interval: interval}
}

// Run polls immediately, then every interval, until ctx is cancelled. A restart
// therefore refreshes rates right away rather than waiting a full interval.
func (p *Poller) Run(ctx context.Context) {
	if len(p.edges) == 0 {
		log.Println("bridge: no edges configured; poller idle")
		return
	}
	log.Printf("bridge: polling %d edge(s) every %s", len(p.edges), p.interval)

	p.pollAll(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollAll(ctx)
		}
	}
}

// pollAll samples every configured edge once. A failure on one edge is logged
// and does not stop the others.
func (p *Poller) pollAll(ctx context.Context) {
	for _, e := range p.edges {
		if err := p.pollOne(ctx, e); err != nil {
			log.Printf("bridge: %s: %v", e.product(), err)
		}
	}
}

// pollOne observes a single edge and writes both the latest row and the
// daily-close candle for the observation's UTC day.
func (p *Poller) pollOne(ctx context.Context, e Edge) error {
	price, ts, err := p.src.Ticker(ctx, e.product())
	if err != nil {
		return err
	}

	ev := provider.PriceEvent{
		Base:      e.Base,
		Quote:     e.Quote,
		Price:     price,
		Timestamp: ts,
		Source:    model.SourceCoinbase,
	}
	if err := p.latest.Upsert(ev); err != nil {
		return fmt.Errorf("latest upsert: %w", err)
	}

	// Daily close: one 1d candle per UTC day. A stablecoin barely moves within a
	// day, so a single spot observation stands in for OHLC (open=high=low=close).
	day := ts.UTC().Truncate(24 * time.Hour)
	candle := model.PriceCandle{
		Base:     e.Base,
		Quote:    e.Quote,
		Interval: model.Interval1d,
		OpenTime: day,
		Open:     price,
		High:     price,
		Low:      price,
		Close:    price,
		Volume:   0,
		Source:   model.SourceCoinbase,
	}
	if err := p.candles.UpsertBatch([]model.PriceCandle{candle}); err != nil {
		return fmt.Errorf("candle upsert: %w", err)
	}

	log.Printf("bridge: %s = %.6f (%s)", e.product(), price, day.Format("2006-01-02"))
	return nil
}

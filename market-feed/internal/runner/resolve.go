// Package runner is the L3 planning + execution layer. It turns declarative job
// specs (from the jobs package) into running work, and validates each job
// against its provider's capabilities at startup rather than failing at runtime.
//
// Capabilities are matched structurally: a stream job needs a provider.Streamer,
// a poll job a provider.TickerFetcher, a backfill job a provider.OHLCFetcher.
// Providers advertise only what they can do, so "poll from binance" (no ticker)
// is rejected here with a clear message before any goroutine starts.
package runner

import (
	"fmt"
	"strings"

	"market-feed/internal/config"
	"market-feed/internal/model"
	"market-feed/internal/provider"
)

// Providers holds the concrete drivers, keyed for capability lookup by name.
// A single provider name may back different drivers per capability (Binance
// streams over WS but fetches OHLC over REST).
type Providers struct {
	binanceWS   *provider.BinanceWS
	binanceREST *provider.BinanceREST
	krakenREST  *provider.KrakenREST
}

// NewProviders wires the drivers from infra config (base URLs).
func NewProviders(cfg config.Config) *Providers {
	return &Providers{
		binanceWS:   provider.NewBinanceWS(cfg.BinanceWSBase),
		binanceREST: provider.NewBinanceREST(cfg.BinanceRESTBase),
		krakenREST:  provider.NewKrakenREST(cfg.KrakenRESTBase),
	}
}

// Streamer returns the streaming driver for name, or an error if that provider
// cannot stream.
func (p *Providers) Streamer(name string) (provider.Streamer, error) {
	switch strings.ToLower(name) {
	case "binance":
		return p.binanceWS, nil
	default:
		return nil, fmt.Errorf("provider %q has no streaming capability", name)
	}
}

// OHLCFetcher returns the OHLC driver for name, or an error if that provider
// cannot fetch candles.
func (p *Providers) OHLCFetcher(name string) (provider.OHLCFetcher, error) {
	switch strings.ToLower(name) {
	case "binance":
		return p.binanceREST, nil
	case "kraken":
		return p.krakenREST, nil
	default:
		return nil, fmt.Errorf("provider %q has no OHLC capability", name)
	}
}

// TickerFetcher returns the ticker driver for name, or an error if that provider
// cannot fetch a spot price.
func (p *Providers) TickerFetcher(name string) (provider.TickerFetcher, error) {
	switch strings.ToLower(name) {
	case "kraken":
		return p.krakenREST, nil
	default:
		return nil, fmt.Errorf("provider %q has no ticker capability", name)
	}
}

// SourceFor maps a provider name to its stored model.Source tag.
func SourceFor(name string) model.Source {
	switch strings.ToLower(name) {
	case "binance":
		return model.SourceBinance
	case "kraken":
		return model.SourceKraken
	default:
		return model.Source(strings.ToLower(name))
	}
}

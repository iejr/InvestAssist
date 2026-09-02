package runner

import (
	"testing"

	"market-feed/internal/config"
	"market-feed/internal/model"
)

func TestResolve_CapabilityMatching(t *testing.T) {
	p := NewProviders(config.Config{})

	// Binance streams and fetches OHLC, but has no ticker capability.
	if _, err := p.Streamer("binance"); err != nil {
		t.Errorf("binance should stream: %v", err)
	}
	if _, err := p.OHLCFetcher("binance"); err != nil {
		t.Errorf("binance should fetch OHLC: %v", err)
	}
	if _, err := p.TickerFetcher("binance"); err == nil {
		t.Errorf("binance should NOT have ticker capability")
	}

	// Kraken fetches OHLC and ticker, but does not stream (yet).
	if _, err := p.TickerFetcher("kraken"); err != nil {
		t.Errorf("kraken should have ticker: %v", err)
	}
	if _, err := p.OHLCFetcher("kraken"); err != nil {
		t.Errorf("kraken should fetch OHLC: %v", err)
	}
	if _, err := p.Streamer("kraken"); err == nil {
		t.Errorf("kraken should NOT stream")
	}

	// Unknown provider is rejected for every capability.
	if _, err := p.OHLCFetcher("bogus"); err == nil {
		t.Errorf("unknown provider should be rejected")
	}
}

func TestSourceFor(t *testing.T) {
	if SourceFor("binance") != model.SourceBinance {
		t.Errorf("binance source wrong")
	}
	if SourceFor("KRAKEN") != model.SourceKraken {
		t.Errorf("kraken source should be case-insensitive")
	}
}

package normalize

import "testing"

func TestSymbol(t *testing.T) {
	tests := []struct {
		in         string
		wantBase   string
		wantQuote  string
		wantOK     bool
	}{
		{"BTCUSDT", "BTC", "USDT", true},
		{"btcusdt", "BTC", "USDT", true}, // lowercased input
		{"ETHUSDT", "ETH", "USDT", true},
		{"BTCUSDC", "BTC", "USDC", true},
		{"USDCUSDT", "USDC", "USDT", true}, // longest-quote-first: splits on USDT, not USDC
		{"BTCUSD", "BTC", "USD", true},
		{"ETHBTC", "ETH", "BTC", true},
		{"  BTCUSDT  ", "BTC", "USDT", true}, // trimmed
		{"USDT", "", "", false},             // quote-only, no base
		{"XYZ", "", "", false},              // unknown quote
		{"", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			base, quote, ok := Symbol(tt.in)
			if ok != tt.wantOK || base != tt.wantBase || quote != tt.wantQuote {
				t.Errorf("Symbol(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.in, base, quote, ok, tt.wantBase, tt.wantQuote, tt.wantOK)
			}
		})
	}
}

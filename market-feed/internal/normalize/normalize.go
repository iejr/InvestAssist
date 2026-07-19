package normalize

import "strings"

// knownQuotes lists the quote assets we recognize, longest-first so that a
// symbol like "USDCUSDT" splits on "USDT" (quote) rather than "USDC".
var knownQuotes = []string{"USDT", "USDC", "FDUSD", "USD", "BTC", "ETH", "EUR"}

// Symbol splits a Binance symbol (e.g. "BTCUSDT") into its base and quote
// assets ("BTC", "USDT"). ok is false if no known quote suffix matches.
func Symbol(sym string) (base, quote string, ok bool) {
	s := strings.ToUpper(strings.TrimSpace(sym))
	for _, q := range knownQuotes {
		if len(s) > len(q) && strings.HasSuffix(s, q) {
			return s[:len(s)-len(q)], q, true
		}
	}
	return "", "", false
}

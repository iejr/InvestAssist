package pricing

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"strategy-server-go/internal/market"
)

// rawField builds one HGETALL field/value pair as market-feed would write it.
func rawField(t *testing.T, base, quote string, price float64, updatedAt time.Time) (field, value string) {
	b, err := json.Marshal(market.LatestPrice{Price: price, Source: "binance", UpdatedAt: updatedAt})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return base + "/" + quote, string(b)
}

func TestBuildGraph_LinksBothDirections(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	field, val := rawField(t, "BTC", "USDT", 50000, now)

	g, err := buildGraph(map[string]string{field: val})
	if err != nil {
		t.Fatalf("buildGraph: %v", err)
	}

	path, ok := g.route("BTC", "USDT")
	if !ok || len(path) != 1 || path[0].rate() != 50000 {
		t.Fatalf("BTC->USDT route = %+v, ok=%v", path, ok)
	}

	path, ok = g.route("USDT", "BTC")
	if !ok || len(path) != 1 || path[0].rate() != 1.0/50000 {
		t.Fatalf("USDT->BTC route = %+v, ok=%v", path, ok)
	}
}

func TestBuildGraph_SkipsNonPositivePrice(t *testing.T) {
	field, val := rawField(t, "BTC", "USDT", 0, time.Now())
	g, err := buildGraph(map[string]string{field: val})
	if err != nil {
		t.Fatalf("buildGraph: %v", err)
	}
	if _, ok := g.route("BTC", "USDT"); ok {
		t.Fatalf("expected no route for a non-positive price")
	}
}

func TestBuildGraph_SkipsMalformedField(t *testing.T) {
	g, err := buildGraph(map[string]string{"not-a-pair": `{"price":1}`})
	if err != nil {
		t.Fatalf("buildGraph: %v", err)
	}
	if len(g.bases()) != 0 {
		t.Fatalf("expected no bases from a malformed field, got %v", g.bases())
	}
}

func TestGraphBases_SortedDistinctEndpoints(t *testing.T) {
	now := time.Now()
	f1, v1 := rawField(t, "BTC", "USDT", 50000, now)
	f2, v2 := rawField(t, "USDT", "USD", 1, now)

	g, err := buildGraph(map[string]string{f1: v1, f2: v2})
	if err != nil {
		t.Fatalf("buildGraph: %v", err)
	}

	got := g.bases()
	want := []string{"BTC", "USD", "USDT"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bases() = %v, want %v", got, want)
	}
}

func TestGraphRoute_MultiHop(t *testing.T) {
	now := time.Now()
	f1, v1 := rawField(t, "BTC", "USDT", 50000, now)
	f2, v2 := rawField(t, "USDT", "USD", 1.0005, now)

	g, err := buildGraph(map[string]string{f1: v1, f2: v2})
	if err != nil {
		t.Fatalf("buildGraph: %v", err)
	}

	path, ok := g.route("BTC", "USD")
	if !ok || len(path) != 2 {
		t.Fatalf("BTC->USD route = %+v, ok=%v", path, ok)
	}
	got := path[0].rate() * path[1].rate()
	want := 50000 * 1.0005
	if got != want {
		t.Fatalf("BTC->USD combined rate = %v, want %v", got, want)
	}
}

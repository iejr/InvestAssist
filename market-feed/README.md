# market-feed

Market data ingestion service for InvestAssist. It streams and polls crypto /
stablecoin prices, samples them into OHLC candles, and backfills historical
candles — writing `latest_prices` to the same Redis that `strategy-server-go`
reads for live valuation, and `price_candles` to the same Postgres it reads
for history.

Consumers never talk to an exchange; they only read `latest_prices` (Redis)
and `price_candles` (Postgres).

## Architecture

Layered and provider-agnostic.

```
sources ─┐                         ┌─ latest-writer ─→ latest_prices (Redis)
 stream  ├─→ PriceEvent ─→ bus ─→ ─┤
 poll   ─┘                         └─ sampler(s) ────→ price_candles (Postgres)
                                        (one per interval)

backfill (offline, one-shot) ────────────────────────→ price_candles
```

- **Jobs (L4)** — a declarative YAML list of `stream` / `poll` / `backfill` jobs,
  each naming a provider and a symbol.
- **Runner (L3)** — validates each job against its provider's capability at
  startup, then runs it: stream/poll publish to an in-memory bus; sinks are a
  coalesced latest-writer plus one sampler per candle interval.
- **Providers** — capability interfaces (`Streamer`, `OHLCFetcher`,
  `TickerFetcher`). Binance backs live crypto; Kraken backs the stablecoin→USD
  bridge (never assumed 1:1).

## Quick start

```bash
cp .env.example .env        # edit DATABASE_URL etc.
go run ./cmd/marketd        # serve: runs the stream + poll jobs, blocks
```

With no `MF_JOBS_FILE` set, a built-in default runs: stream `MF_SYMBOLS` from
Binance + a daily Kraken poll and 1d backfill for USDT/USD and USDC/USD.

## Configuration

All config is via environment (see [`.env.example`](.env.example)):

| Var | Purpose | Default |
|-----|---------|---------|
| `DATABASE_URL` | Postgres DSN for `price_candles` (shared with server-go) | local dev DSN |
| `MF_REDIS_ADDR` | Redis address for `latest_prices` (shared with strategy-server-go) | `localhost:6379` |
| `MF_REDIS_DB` / `MF_REDIS_PASSWORD` | optional Redis DB index / auth | `0` / none |
| `MF_SYMBOLS` | Binance symbols to stream | `BTCUSDT` |
| `MF_SAMPLE_INTERVALS` | candle intervals, e.g. `1m,5m` | `1m` |
| `MF_LATEST_COALESCE` | throttle latest_prices writes per edge | `1s` |
| `MF_JOBS_FILE` | path to the YAML job list | built-in default |
| `MF_BINANCE_WS` / `MF_BINANCE_REST` | Binance endpoints | public |
| `MF_KRAKEN_REST` | Kraken endpoint | public |

Sample intervals must be one of `1s,5s,1m,5m,1h,1d`; other values are skipped.

## Jobs

The job list lives in YAML (see [`jobs.example.yaml`](jobs.example.yaml)). Three kinds:

- `stream` — one provider connection carrying N symbols → live latest + candles.
- `poll` — one edge's spot price on a slow timer (`every`) → latest + a
  pass-through candle. `every` must be a valid candle interval.
- `backfill` — real OHLC candles over a window; run offline (see below).

## Backfill

One-off, driven externally (cron), not part of `serve`.

```bash
# Run all backfill jobs from the jobs file over their configured lookback:
go run ./cmd/marketd -backfill

# Ad-hoc single symbol over an explicit window:
go run ./cmd/marketd -backfill -symbol BTCUSDT -interval 1m \
    -from 2026-07-01 -to 2026-07-02
```

Flag precedence is **explicit CLI flag > YAML value > default**; `-from`/`-to`
override the window in both modes. `scripts/backfill_daily.sh` wraps the
jobs-mode run for a daily cron (previous UTC day).

## Development

```bash
go build ./...
go vet ./...
go test ./...
```

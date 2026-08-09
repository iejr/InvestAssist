#!/usr/bin/env bash
#
# backfill_daily.sh — backfill yesterday's candles for every configured symbol.
#
# Intended to run once a day (e.g. from cron shortly after 00:00 UTC). Fetches
# the full previous UTC day at $INTERVAL and overwrites existing rows so re-runs
# are self-healing. All day math is in UTC to match candle open_time.
#
# Usage:
#   scripts/backfill_daily.sh                 # yesterday, symbols from .env
#   INTERVAL=1m scripts/backfill_daily.sh     # override interval
#   scripts/backfill_daily.sh 2026-07-01      # a specific UTC day instead
#
set -euo pipefail

# --- locate the market-feed root (parent of this script's dir) --------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

# --- make Go available under cron's minimal PATH ----------------------------
export PATH="$PATH:/usr/local/go/bin"

# --- load configuration (.env) ----------------------------------------------
if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

INTERVAL="${INTERVAL:-1m}"
SYMBOLS="${MF_SYMBOLS:-BTCUSDT}"

# --- compute the UTC day window ---------------------------------------------
# Optional first arg = an explicit YYYY-MM-DD day; default is yesterday (UTC).
if [[ $# -ge 1 ]]; then
  FROM="$1"
  TO="$(date -u -d "$FROM + 1 day" +%Y-%m-%d)"
else
  FROM="$(date -u -d 'yesterday' +%Y-%m-%d)"
  TO="$(date -u +%Y-%m-%d)"   # today 00:00Z, exclusive end -> full previous day
fi

log() { echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*"; }

log "backfill start: interval=$INTERVAL window=${FROM}..${TO} (UTC) symbols=[$SYMBOLS]"

# --- backfill each symbol ----------------------------------------------------
rc=0
IFS=',' read -ra SYMS <<< "$SYMBOLS"
for raw in "${SYMS[@]}"; do
  sym="$(echo "$raw" | tr -d '[:space:]')"
  [[ -z "$sym" ]] && continue

  log "backfilling $sym ..."
  if go run ./cmd/marketd \
        -backfill \
        -symbol "$sym" \
        -interval "$INTERVAL" \
        -from "$FROM" \
        -to "$TO" \
        -override=true; then
    log "backfilled $sym OK"
  else
    log "ERROR backfilling $sym (continuing)"
    rc=1
  fi
done

log "backfill done (exit $rc)"
exit "$rc"

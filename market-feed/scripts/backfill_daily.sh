#!/usr/bin/env bash
#
# backfill_daily.sh — backfill yesterday's candles for every backfill job.
#
# Intended to run once a day (e.g. from cron shortly after 00:00 UTC). Runs
# `marketd -backfill` in jobs mode: it loads the jobs file (MF_JOBS_FILE, or the
# built-in default) and runs every `backfill` job, each at its own configured
# interval. Only the window is overridden here — to the full previous UTC day,
# [yesterday 00:00Z, today 00:00Z) — via -from/-to, which take precedence over
# each job's lookback. -override=true overwrites existing rows so re-runs are
# self-healing. All day math is in UTC to match candle open_time.
#
# Usage:
#   scripts/backfill_daily.sh                 # yesterday's UTC day
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
# Provides DATABASE_URL, provider endpoints, and MF_JOBS_FILE (which backfill
# jobs to run). marketd reads these from the environment.
if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

# --- compute the UTC day window ---------------------------------------------
# Optional first arg = an explicit YYYY-MM-DD day; default is yesterday (UTC).
# TO is the next day's 00:00Z: an exclusive end covering the full previous day.
if [[ $# -ge 1 ]]; then
  FROM="$1"
  TO="$(date -u -d "$FROM + 1 day" +%Y-%m-%d)"
else
  FROM="$(date -u -d 'yesterday' +%Y-%m-%d)"
  TO="$(date -u +%Y-%m-%d)"
fi

log() { echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*"; }

log "backfill start: window=${FROM}..${TO} (UTC), jobs from ${MF_JOBS_FILE:-<built-in default>}"

# --- run every backfill job over the day window ------------------------------
# No -symbol: jobs mode. No -interval: each job keeps its own YAML interval.
# -from/-to override the per-job lookback so all jobs target the same UTC day.
rc=0
if go run ./cmd/marketd \
      -backfill \
      -from "$FROM" \
      -to "$TO" \
      -override=true; then
  log "backfill done OK"
else
  log "ERROR: backfill failed"
  rc=1
fi

exit "$rc"

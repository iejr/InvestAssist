-- strategy-server-go's as-of price lookup is granularity-agnostic (it takes
-- the newest candle of ANY interval at-or-before a timestamp), so it filters
-- on (base, quote, open_time) without `interval`. idx_candle_lookup can't
-- serve that efficiently since `interval` sits ahead of open_time in its key.
CREATE INDEX IF NOT EXISTS idx_candle_asof
    ON price_candles (base, quote, open_time DESC);

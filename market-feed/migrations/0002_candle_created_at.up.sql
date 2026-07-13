-- Track when each candle row was written (append-only; distinct from open_time,
-- which is market time). Helps trace ingestion issues.
ALTER TABLE price_candles
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- market-feed initial schema.
-- Fact tables only: latest edge values and append-only OHLC observations.
-- Prices are stored in their native quote currency (e.g. BTC/USDT); canonical
-- USD and display-currency values are derived on read, never stored here.

-- Latest edge values: one row per (base, quote), always upserted.
CREATE TABLE IF NOT EXISTS latest_prices (
    base       TEXT NOT NULL,
    quote      TEXT NOT NULL,
    price      DOUBLE PRECISION NOT NULL,
    source     TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (base, quote)
);

-- Historical OHLC: append-only, native quote, real observations only.
CREATE TABLE IF NOT EXISTS price_candles (
    id        BIGSERIAL PRIMARY KEY,
    base      TEXT NOT NULL,
    quote     TEXT NOT NULL,
    interval  TEXT NOT NULL CHECK (interval IN ('1s','5s','1m','5m','1h','1d')),
    open_time TIMESTAMPTZ NOT NULL,
    open      DOUBLE PRECISION NOT NULL,
    high      DOUBLE PRECISION NOT NULL,
    low       DOUBLE PRECISION NOT NULL,
    close     DOUBLE PRECISION NOT NULL,
    volume    DOUBLE PRECISION NOT NULL DEFAULT 0,
    source    TEXT NOT NULL,
    CONSTRAINT uq_candle UNIQUE (base, quote, interval, open_time, source)
);

CREATE INDEX IF NOT EXISTS idx_candle_lookup
    ON price_candles (base, quote, interval, open_time DESC);

-- Recreates latest_prices as it was before the Redis cutover (see
-- 0001_init.up.sql). Rollback only restores the schema, not the data.
CREATE TABLE IF NOT EXISTS latest_prices (
    base       TEXT NOT NULL,
    quote      TEXT NOT NULL,
    price      DOUBLE PRECISION NOT NULL,
    source     TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (base, quote)
);

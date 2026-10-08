-- strategy-server-go's own schema. These two tables already live in
-- DATABASE_URL (never in MF_HISTORY_DATABASE_URL), so this file just gives
-- the surviving Postgres instance a tracked-SQL record, mirroring
-- market-feed/migrations/. GORM AutoMigrate (internal/db/db.go) remains the
-- dev-convenience path and stays authoritative for day-to-day column tweaks
-- until this file is wired into an actual migration runner.

CREATE TABLE IF NOT EXISTS strategies (
    id         UUID NOT NULL PRIMARY KEY,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL,
    base       TEXT NOT NULL,
    quote      TEXT NOT NULL,
    start_date TIMESTAMPTZ NOT NULL,
    interval   TEXT NOT NULL,
    increment  DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_strategies_deleted_at ON strategies (deleted_at);

CREATE TABLE IF NOT EXISTS transactions (
    id           UUID NOT NULL PRIMARY KEY,
    strategy_id  UUID NOT NULL,
    type         TEXT NOT NULL,
    shares       DOUBLE PRECISION NOT NULL,
    price        DOUBLE PRECISION NOT NULL,
    fee_currency TEXT NOT NULL DEFAULT '',
    fee_amount   DOUBLE PRECISION NOT NULL DEFAULT 0,
    timestamp    TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_transactions_strategy_id ON transactions (strategy_id);

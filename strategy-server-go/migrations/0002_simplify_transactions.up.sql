-- Simplify transactions to a spent/gained ledger: drop the fee-modeling
-- columns (type, shares, price, fee_currency, fee_amount) in favor of two
-- generic legs. Net amounts only; fees etc. are folded in by the user.

ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS spent_symbol  TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS spent_amount  DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gained_symbol TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS gained_amount DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS memo          TEXT NOT NULL DEFAULT '';

ALTER TABLE transactions
    DROP COLUMN IF EXISTS type,
    DROP COLUMN IF EXISTS shares,
    DROP COLUMN IF EXISTS price,
    DROP COLUMN IF EXISTS fee_currency,
    DROP COLUMN IF EXISTS fee_amount;

ALTER TABLE transactions ALTER COLUMN spent_symbol DROP DEFAULT;
ALTER TABLE transactions ALTER COLUMN spent_amount DROP DEFAULT;
ALTER TABLE transactions ALTER COLUMN gained_symbol DROP DEFAULT;
ALTER TABLE transactions ALTER COLUMN gained_amount DROP DEFAULT;

-- Switch the values this service owns outright from DOUBLE PRECISION (binary
-- float64 — the same representation Go's float64 uses, so no exactness was
-- ever gained by crossing the DB boundary) to NUMERIC (exact decimal). This
-- stops future float round-tripping; it does not retroactively "clean up"
-- any rounding already baked into existing rows by a prior float64 write —
-- the cast preserves each row's current value exactly, whatever it is.
-- Plain NUMERIC (no fixed precision/scale): Postgres stores the value
-- exactly as given, and rounding for display happens in the app layer, not
-- here.
ALTER TABLE strategies
    ALTER COLUMN increment TYPE NUMERIC;

ALTER TABLE transactions
    ALTER COLUMN spent_amount TYPE NUMERIC,
    ALTER COLUMN gained_amount TYPE NUMERIC;

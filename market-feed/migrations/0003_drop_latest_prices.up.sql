-- latest_prices has moved to Redis (single HASH, field "{base}/{quote}").
-- Do not run this until strategy-server-go reads from Redis in every
-- environment and that has been verified.
DROP TABLE IF EXISTS latest_prices;

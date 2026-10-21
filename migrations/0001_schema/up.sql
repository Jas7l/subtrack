-- 0001_schema: the schema every SubTrack table lives in. The server's DSN pins
-- search_path to it (internal/pg/config.go), so it must exist before the
-- server starts.
CREATE SCHEMA IF NOT EXISTS subtrack;

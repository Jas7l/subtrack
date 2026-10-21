-- 0003_currencies: currencies money can be recorded in, with the current rate
-- to roubles that the admin maintains by hand.
--
-- Every amount in the system is stored as NUMERIC(14,2) next to a currency
-- code that references this table, so a rate is always available. Reports
-- convert to roubles as amount * rate_to_rub using the CURRENT rate; rate
-- history is intentionally not kept.

CREATE TABLE subtrack.currencies (
    code         CHAR(3)        PRIMARY KEY CHECK (code ~ '^[A-Z]{3}$'),
    name         TEXT           NOT NULL,
    -- How many roubles one unit of this currency costs.
    rate_to_rub  NUMERIC(18,6)  NOT NULL CHECK (rate_to_rub > 0),
    updated_at   TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_by   BIGINT         REFERENCES subtrack.users(id) ON DELETE SET NULL,
    -- The rouble is the reporting currency; its rate is fixed at 1.
    CONSTRAINT currencies_rub_rate_check CHECK (code <> 'RUB' OR rate_to_rub = 1)
);

INSERT INTO subtrack.currencies (code, name, rate_to_rub) VALUES ('RUB', 'Российский рубль', 1);

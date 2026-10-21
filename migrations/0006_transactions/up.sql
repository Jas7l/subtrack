-- 0006_transactions: payment history.
--
-- The scheduler creates a pending row on a subscription's next_payment_date;
-- the admin marks it paid (or failed) after paying. The unique key on
-- (subscription_id, payment_date) keeps a second scheduler run from creating
-- the same payment twice.

CREATE TABLE subtrack.transactions (
    id               BIGSERIAL      PRIMARY KEY,
    subscription_id  BIGINT         NOT NULL REFERENCES subtrack.subscriptions(id),
    amount           NUMERIC(14,2)  NOT NULL CHECK (amount >= 0),
    currency         CHAR(3)        NOT NULL REFERENCES subtrack.currencies(code),
    payment_date     DATE           NOT NULL,
    status           TEXT           NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'failed')),
    payment_method   TEXT           NOT NULL DEFAULT '',
    invoice_number   TEXT           NOT NULL DEFAULT '',
    paid_at          TIMESTAMPTZ,
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),

    CONSTRAINT transactions_paid_check CHECK ((status = 'paid') = (paid_at IS NOT NULL))
);

CREATE UNIQUE INDEX transactions_subscription_date_key ON subtrack.transactions (subscription_id, payment_date);
CREATE INDEX        transactions_payment_date_idx      ON subtrack.transactions (payment_date);
CREATE INDEX        transactions_status_idx            ON subtrack.transactions (status);

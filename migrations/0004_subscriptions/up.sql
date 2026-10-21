-- 0004_subscriptions: the subscription registry and who holds each licence.
--
-- Subscriptions are soft-deleted (deleted_at) so payment history and past
-- assignments keep pointing at a real row.
--
-- seats_used is a counter kept by the service in the same transaction that
-- creates or revokes an assignment; the CHECK below stops it from going past
-- seats_total. Enterprise subscriptions are unlimited: seats_total is NULL.

CREATE TABLE subtrack.subscriptions (
    id                 BIGSERIAL      PRIMARY KEY,
    name               TEXT           NOT NULL,
    vendor             TEXT           NOT NULL DEFAULT '',
    category           TEXT           NOT NULL DEFAULT '',
    type               TEXT           NOT NULL CHECK (type IN ('individual', 'group', 'enterprise')),
    cost               NUMERIC(14,2)  NOT NULL CHECK (cost >= 0),
    currency           CHAR(3)        NOT NULL REFERENCES subtrack.currencies(code),
    billing_period     TEXT           NOT NULL CHECK (billing_period IN ('month', 'quarter', 'year')),
    start_date         DATE           NOT NULL,
    end_date           DATE,
    next_payment_date  DATE,
    status             TEXT           NOT NULL DEFAULT 'active'
                                      CHECK (status IN ('active', 'expiring_soon', 'expired', 'cancelled')),
    department_id      BIGINT         REFERENCES subtrack.departments(id) ON DELETE SET NULL,
    seats_total        INT            CHECK (seats_total > 0),
    seats_used         INT            NOT NULL DEFAULT 0 CHECK (seats_used >= 0),
    created_by         BIGINT         REFERENCES subtrack.users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    deleted_at         TIMESTAMPTZ,

    CONSTRAINT subscriptions_dates_check   CHECK (end_date IS NULL OR end_date >= start_date),
    CONSTRAINT subscriptions_seats_check   CHECK (seats_total IS NULL OR seats_used <= seats_total),
    -- Only enterprise subscriptions are unlimited; the others must say how
    -- many seats were bought.
    CONSTRAINT subscriptions_seats_type_check CHECK ((type = 'enterprise') = (seats_total IS NULL))
);

CREATE INDEX subscriptions_department_id_idx     ON subtrack.subscriptions (department_id)     WHERE deleted_at IS NULL;
CREATE INDEX subscriptions_status_idx            ON subtrack.subscriptions (status)            WHERE deleted_at IS NULL;
CREATE INDEX subscriptions_end_date_idx          ON subtrack.subscriptions (end_date)          WHERE deleted_at IS NULL;
CREATE INDEX subscriptions_next_payment_date_idx ON subtrack.subscriptions (next_payment_date) WHERE deleted_at IS NULL;

CREATE TABLE subtrack.subscription_assignments (
    id               BIGSERIAL    PRIMARY KEY,
    subscription_id  BIGINT       NOT NULL REFERENCES subtrack.subscriptions(id),
    user_id          BIGINT       NOT NULL REFERENCES subtrack.users(id),
    status           TEXT         NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    assigned_by      BIGINT       REFERENCES subtrack.users(id) ON DELETE SET NULL,
    assigned_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    revoked_by       BIGINT       REFERENCES subtrack.users(id) ON DELETE SET NULL,
    revoked_at       TIMESTAMPTZ,

    CONSTRAINT subscription_assignments_revoked_check CHECK ((status = 'revoked') = (revoked_at IS NOT NULL))
);

-- A user holds at most one live licence per subscription; revoked rows stay
-- as history, so the same pair may be assigned again later.
CREATE UNIQUE INDEX subscription_assignments_active_key
    ON subtrack.subscription_assignments (subscription_id, user_id) WHERE status = 'active';
CREATE INDEX subscription_assignments_user_id_idx
    ON subtrack.subscription_assignments (user_id) WHERE status = 'active';

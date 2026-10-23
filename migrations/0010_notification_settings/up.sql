-- 0010_notification_settings: how early the scheduler warns about a
-- subscription's end and its next payment. The admin edits the values over
-- the API; the scheduler reads them at the start of every run.
--
-- The table holds exactly one row: its key can only be TRUE.

CREATE TABLE subtrack.notification_settings (
    id             BOOLEAN      PRIMARY KEY DEFAULT TRUE CHECK (id),
    -- end_date within this many days (today included) -> expiring_soon.
    expiring_days  INT          NOT NULL DEFAULT 7 CHECK (expiring_days BETWEEN 0 AND 365),
    -- next_payment_date within this many days -> a reminder to the admins.
    payment_days   INT          NOT NULL DEFAULT 3 CHECK (payment_days BETWEEN 0 AND 365),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_by     BIGINT       REFERENCES subtrack.users(id) ON DELETE SET NULL
);

INSERT INTO subtrack.notification_settings DEFAULT VALUES;

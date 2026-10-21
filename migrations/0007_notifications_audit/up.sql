-- 0007_notifications_audit: in-app notifications, the dedup log the scheduler
-- writes before sending, and the audit trail of who changed what.

CREATE TABLE subtrack.notifications (
    id          BIGSERIAL    PRIMARY KEY,
    user_id     BIGINT       NOT NULL REFERENCES subtrack.users(id) ON DELETE CASCADE,
    type        TEXT         NOT NULL CHECK (type IN ('subscription_expiring', 'subscription_expired', 'payment_due',
                                                      'request_created', 'request_approved', 'request_rejected')),
    title       TEXT         NOT NULL,
    body        TEXT         NOT NULL DEFAULT '',
    is_read     BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX notifications_user_id_idx ON subtrack.notifications (user_id, created_at DESC);
CREATE INDEX notifications_unread_idx  ON subtrack.notifications (user_id) WHERE NOT is_read;

-- One row per event already notified, keyed like "sub_5_expiring_2025-01-15".
-- The scheduler inserts the key with ON CONFLICT DO NOTHING and only sends
-- when the insert actually happened, so a double run sends nothing twice.
CREATE TABLE subtrack.notification_log (
    key      TEXT         PRIMARY KEY,
    sent_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE subtrack.audit_log (
    id           BIGSERIAL    PRIMARY KEY,
    -- NULL for actions taken by the scheduler.
    actor_id     BIGINT       REFERENCES subtrack.users(id) ON DELETE SET NULL,
    action       TEXT         NOT NULL,   -- e.g. "assignment.revoke"
    entity_type  TEXT         NOT NULL,   -- e.g. "subscription"
    entity_id    BIGINT,
    details      JSONB        NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_log_created_at_idx ON subtrack.audit_log (created_at DESC);
CREATE INDEX audit_log_entity_idx     ON subtrack.audit_log (entity_type, entity_id);
CREATE INDEX audit_log_actor_id_idx   ON subtrack.audit_log (actor_id);

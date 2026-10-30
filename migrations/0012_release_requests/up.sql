-- 0012_release_requests: a licence holder asking to give the licence back.
--
-- State machine, enforced by the service:
--
--   pending ──approve──▶ approved   (the licence is revoked in the same transaction)
--      │
--      ├────reject───▶ rejected
--      │
--      └──requester cancels──▶ cancelled
--
-- Whoever manages the subscription decides: a manager of its department or
-- the admin. The department is not stored here: it is the subscription's
-- one at the time of the decision.
--
-- A licence revoked another way while its request waits (revoked directly,
-- the subscription cancelled or deleted) closes the request as approved by
-- whoever revoked it; blocking the requester cancels it. So a pending
-- request always points at an active licence.
--
-- requester_id is always the licence holder (only the holder asks).

CREATE TABLE subtrack.release_requests (
    id             BIGSERIAL    PRIMARY KEY,
    assignment_id  BIGINT       NOT NULL REFERENCES subtrack.subscription_assignments(id),
    requester_id   BIGINT       NOT NULL REFERENCES subtrack.users(id),
    reason         TEXT         NOT NULL DEFAULT '',
    status         TEXT         NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    decided_by     BIGINT       REFERENCES subtrack.users(id) ON DELETE SET NULL,
    comment        TEXT         NOT NULL DEFAULT '',
    decided_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    -- A decision has a moment; a pending or cancelled request has none.
    CONSTRAINT release_requests_decided_check CHECK ((status IN ('approved', 'rejected')) = (decided_at IS NOT NULL))
);

-- At most one request waits per licence; decided ones stay as history.
CREATE UNIQUE INDEX release_requests_pending_key
    ON subtrack.release_requests (assignment_id) WHERE status = 'pending';
CREATE INDEX release_requests_requester_id_idx ON subtrack.release_requests (requester_id, created_at DESC);
CREATE INDEX release_requests_status_idx ON subtrack.release_requests (status);

-- Notifications about them: three new types and a link to the request.
ALTER TABLE subtrack.notifications
    DROP CONSTRAINT notifications_type_check,
    ADD CONSTRAINT notifications_type_check
        CHECK (type IN ('subscription_expiring', 'subscription_expired', 'payment_due',
                        'request_created', 'request_approved', 'request_rejected',
                        'release_created', 'release_approved', 'release_rejected')),
    DROP CONSTRAINT notifications_entity_type_check,
    ADD CONSTRAINT notifications_entity_type_check
        CHECK (entity_type IN ('request', 'subscription', 'transaction', 'release_request'));

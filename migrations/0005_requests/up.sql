-- 0005_requests: requests for a new (or an existing) subscription and their
-- path through approval.
--
-- State machine, enforced by the service:
--
--   pending_manager ──reject──▶ rejected_by_manager
--         │ approve
--         ▼
--   pending_admin ────reject──▶ rejected_by_admin
--         │ approve
--         ▼
--      approved
--
--   pending_manager / pending_admin ──requester cancels──▶ cancelled
--
-- A request made by a manager starts at pending_admin.
--
-- subscription_id is set up front when an existing subscription is requested,
-- or filled in on approval with the subscription that was created.

CREATE TABLE subtrack.requests (
    id                  BIGSERIAL      PRIMARY KEY,
    requester_id        BIGINT         NOT NULL REFERENCES subtrack.users(id),
    -- Department at the time of the request: decides which manager reviews it,
    -- even if the requester moves later.
    department_id       BIGINT         REFERENCES subtrack.departments(id) ON DELETE SET NULL,
    name                TEXT           NOT NULL,
    vendor              TEXT           NOT NULL DEFAULT '',
    justification       TEXT           NOT NULL DEFAULT '',
    estimated_cost      NUMERIC(14,2)  CHECK (estimated_cost >= 0),
    currency            CHAR(3)        NOT NULL DEFAULT 'RUB' REFERENCES subtrack.currencies(code),
    subscription_id     BIGINT         REFERENCES subtrack.subscriptions(id),
    status              TEXT           NOT NULL DEFAULT 'pending_manager'
                                       CHECK (status IN ('pending_manager', 'rejected_by_manager', 'pending_admin',
                                                         'rejected_by_admin', 'approved', 'cancelled')),
    manager_id          BIGINT         REFERENCES subtrack.users(id) ON DELETE SET NULL,
    manager_comment     TEXT           NOT NULL DEFAULT '',
    manager_decided_at  TIMESTAMPTZ,
    admin_id            BIGINT         REFERENCES subtrack.users(id) ON DELETE SET NULL,
    admin_comment       TEXT           NOT NULL DEFAULT '',
    admin_decided_at    TIMESTAMPTZ,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX requests_requester_id_idx ON subtrack.requests (requester_id, created_at DESC);
CREATE INDEX requests_department_status_idx ON subtrack.requests (department_id, status);
CREATE INDEX requests_status_idx ON subtrack.requests (status);

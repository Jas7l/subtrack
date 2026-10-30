-- Notifications of the new types (and links to release requests) would break
-- the old CHECKs: they go first.
DELETE FROM subtrack.notifications
WHERE type IN ('release_created', 'release_approved', 'release_rejected') OR entity_type = 'release_request';

ALTER TABLE subtrack.notifications
    DROP CONSTRAINT IF EXISTS notifications_type_check,
    ADD CONSTRAINT notifications_type_check
        CHECK (type IN ('subscription_expiring', 'subscription_expired', 'payment_due',
                        'request_created', 'request_approved', 'request_rejected')),
    DROP CONSTRAINT IF EXISTS notifications_entity_type_check,
    ADD CONSTRAINT notifications_entity_type_check
        CHECK (entity_type IN ('request', 'subscription', 'transaction'));

DROP TABLE IF EXISTS subtrack.release_requests;

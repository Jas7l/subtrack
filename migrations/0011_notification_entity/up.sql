-- 0011_notification_entity: what a notification points at, so the client
-- can open the request or subscription it is about. Both columns are NULL
-- or both are set. There is no FK: the column holds ids of different
-- tables, and a deleted subscription is only soft-deleted anyway.
--
-- Notifications stored before this migration keep NULLs.

ALTER TABLE subtrack.notifications
    ADD COLUMN entity_type TEXT   CHECK (entity_type IN ('request', 'subscription', 'transaction')),
    ADD COLUMN entity_id   BIGINT,
    ADD CONSTRAINT notifications_entity_check CHECK ((entity_type IS NULL) = (entity_id IS NULL));

ALTER TABLE subtrack.notifications
    DROP CONSTRAINT IF EXISTS notifications_entity_check,
    DROP COLUMN IF EXISTS entity_id,
    DROP COLUMN IF EXISTS entity_type;

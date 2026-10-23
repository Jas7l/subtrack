ALTER TABLE subtrack.users
    ALTER COLUMN full_name TYPE TEXT COLLATE "default",
    ALTER COLUMN email     TYPE TEXT COLLATE "default";

ALTER TABLE subtrack.departments
    ALTER COLUMN name TYPE TEXT COLLATE "default";

DROP INDEX IF EXISTS subtrack.departments_head_id_key;

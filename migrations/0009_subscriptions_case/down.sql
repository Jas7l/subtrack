ALTER TABLE subtrack.subscriptions
    ALTER COLUMN name     TYPE TEXT COLLATE "default",
    ALTER COLUMN vendor   TYPE TEXT COLLATE "default",
    ALTER COLUMN category TYPE TEXT COLLATE "default";

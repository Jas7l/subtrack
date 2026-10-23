-- 0009_subscriptions_case: case-insensitive Cyrillic for the subscription
-- columns people search and filter by.
--
-- Same reason as 0008: under the cluster's "C" locale lower() and ILIKE do not
-- fold Cyrillic, so a search for "бух" would miss "Бухгалтерия" and the
-- category filter "дизайн" would miss "Дизайн". pg_c_utf8 folds all of
-- Unicode and still sorts by code point.

ALTER TABLE subtrack.subscriptions
    ALTER COLUMN name     TYPE TEXT COLLATE pg_c_utf8,
    ALTER COLUMN vendor   TYPE TEXT COLLATE pg_c_utf8,
    ALTER COLUMN category TYPE TEXT COLLATE pg_c_utf8;

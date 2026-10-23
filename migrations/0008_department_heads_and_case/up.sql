-- 0008_department_heads_and_case: one head per department, and case-insensitive
-- text that works for Cyrillic.
--
-- 1. One person heads at most one department. NULLs never collide in a unique
--    index, so any number of departments may be without a head. The service
--    turns a violation into "this user already heads another department" (409).
--    If this fails on existing data, some user already heads two departments:
--    clear head_id on one of them and run it again.
--
-- 2. Under the "C" locale (the cluster default here) lower() and ILIKE only
--    fold ASCII: lower('РАЗРАБОТКА') stays upper-case, so "Разработка" and
--    "РАЗРАБОТКА" would both pass departments_name_key and a search for "диз"
--    would miss "Дизайн". The built-in pg_c_utf8 collation (PostgreSQL 17+,
--    UTF8 databases, no ICU needed) folds all of Unicode and still sorts by
--    code point, like "C". Indexes over these columns, including the lower()
--    unique indexes, are rebuilt automatically.

CREATE UNIQUE INDEX departments_head_id_key ON subtrack.departments (head_id);

ALTER TABLE subtrack.departments
    ALTER COLUMN name TYPE TEXT COLLATE pg_c_utf8;

ALTER TABLE subtrack.users
    ALTER COLUMN full_name TYPE TEXT COLLATE pg_c_utf8,
    ALTER COLUMN email     TYPE TEXT COLLATE pg_c_utf8;

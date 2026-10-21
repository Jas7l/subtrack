-- 0002_users_departments: departments and the people in them.
--
-- departments.head_id and users.department_id point at each other, so the
-- head_id foreign key is added after both tables exist.
--
-- Users are never deleted: blocking a user sets is_active = false, which keeps
-- their history (assignments, requests, audit entries) intact.

CREATE TABLE subtrack.departments (
    id          BIGSERIAL    PRIMARY KEY,
    name        TEXT         NOT NULL,
    head_id     BIGINT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX departments_name_key ON subtrack.departments (lower(name));

CREATE TABLE subtrack.users (
    id             BIGSERIAL    PRIMARY KEY,
    full_name      TEXT         NOT NULL,
    -- Corporate email is the login. Compared case-insensitively.
    email          TEXT         NOT NULL,
    password_hash  TEXT         NOT NULL,
    role           TEXT         NOT NULL CHECK (role IN ('admin', 'manager', 'employee')),
    department_id  BIGINT       REFERENCES subtrack.departments(id) ON DELETE SET NULL,
    is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX users_email_key         ON subtrack.users (lower(email));
CREATE INDEX        users_department_id_idx ON subtrack.users (department_id);

ALTER TABLE subtrack.departments
    ADD CONSTRAINT departments_head_id_fkey
    FOREIGN KEY (head_id) REFERENCES subtrack.users(id) ON DELETE SET NULL;

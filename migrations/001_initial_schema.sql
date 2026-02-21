-- =============================================================================
-- Migration 001: Initial schema
-- Project: Sentinel Authentication Service
-- =============================================================================

-- Enable the pgcrypto extension for gen_random_uuid().
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- -----------------------------------------------------------------------------
-- Table: applications
-- Represents each registered system or application (logical tenant).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS applications (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL UNIQUE,
    slug        VARCHAR(50)  NOT NULL UNIQUE,
    secret_key  VARCHAR(255) NOT NULL,
    is_active   BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- -----------------------------------------------------------------------------
-- Table: users
-- Central user table with lockout tracking.
-- Decision 2026-02-21: lockout_count + lockout_date added to support the
-- "3 lockouts per day = permanent block" rule without requiring an external job.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username        VARCHAR(100) NOT NULL UNIQUE,
    email           VARCHAR(255) NOT NULL UNIQUE,
    password_hash   VARCHAR(255) NOT NULL,
    is_active       BOOLEAN      NOT NULL DEFAULT TRUE,
    must_change_pwd BOOLEAN      NOT NULL DEFAULT FALSE,
    last_login_at   TIMESTAMPTZ,
    failed_attempts INT          NOT NULL DEFAULT 0,
    locked_until    TIMESTAMPTZ,
    lockout_count   INT          NOT NULL DEFAULT 0,
    lockout_date    DATE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- -----------------------------------------------------------------------------
-- Table: permissions
-- Permission catalogue per application. Convention: {module}.{resource}.{action}
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS permissions (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID        NOT NULL REFERENCES applications(id),
    code           VARCHAR(100) NOT NULL,
    description    TEXT,
    scope_type     VARCHAR(20)  NOT NULL
                   CHECK (scope_type IN ('global', 'module', 'resource', 'action')),
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (application_id, code)
);

-- -----------------------------------------------------------------------------
-- Table: cost_centers
-- Cost center catalogue per application.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS cost_centers (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID        NOT NULL REFERENCES applications(id),
    code           VARCHAR(50)  NOT NULL,
    name           VARCHAR(150) NOT NULL,
    is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (application_id, code)
);

-- -----------------------------------------------------------------------------
-- Table: roles
-- Role catalogue per application. is_system = TRUE for the bootstrap admin role.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS roles (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID        NOT NULL REFERENCES applications(id),
    name           VARCHAR(100) NOT NULL,
    description    TEXT,
    is_system      BOOLEAN      NOT NULL DEFAULT FALSE,
    is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (application_id, name)
);

-- -----------------------------------------------------------------------------
-- Table: role_permissions
-- N:M relation between roles and permissions.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- -----------------------------------------------------------------------------
-- Table: user_roles
-- Role assignments to users with optional temporal validity.
-- Active condition: is_active = TRUE AND valid_from <= NOW()
--                  AND (valid_until IS NULL OR valid_until > NOW())
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_roles (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id        UUID        NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    application_id UUID        NOT NULL REFERENCES applications(id),
    granted_by     UUID        NOT NULL REFERENCES users(id),
    valid_from     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    valid_until    TIMESTAMPTZ,
    is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- -----------------------------------------------------------------------------
-- Table: user_permissions
-- Individual permissions assigned directly to users (outside roles).
-- Same validity logic as user_roles.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_permissions (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_id  UUID        NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    application_id UUID        NOT NULL REFERENCES applications(id),
    granted_by     UUID        NOT NULL REFERENCES users(id),
    valid_from     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    valid_until    TIMESTAMPTZ,
    is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- -----------------------------------------------------------------------------
-- Table: user_cost_centers
-- Cost center assignments to users with optional temporal validity.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_cost_centers (
    user_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cost_center_id UUID        NOT NULL REFERENCES cost_centers(id) ON DELETE CASCADE,
    application_id UUID        NOT NULL REFERENCES applications(id),
    granted_by     UUID        NOT NULL REFERENCES users(id),
    valid_from     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    valid_until    TIMESTAMPTZ,
    PRIMARY KEY (user_id, cost_center_id)
);

-- -----------------------------------------------------------------------------
-- Table: refresh_tokens
-- Stores active refresh tokens. Tokens are also mirrored in Redis for fast lookup.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    app_id      UUID        NOT NULL REFERENCES applications(id),
    token_hash  VARCHAR(255) NOT NULL UNIQUE,
    device_info JSONB,
    expires_at  TIMESTAMPTZ  NOT NULL,
    used_at     TIMESTAMPTZ,
    is_revoked  BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- -----------------------------------------------------------------------------
-- Table: audit_logs
-- Immutable audit trail. No UPDATE or DELETE is allowed at the application level.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS audit_logs (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type     VARCHAR(50)  NOT NULL,
    application_id UUID        REFERENCES applications(id),
    user_id        UUID        REFERENCES users(id),
    actor_id       UUID        REFERENCES users(id),
    resource_type  VARCHAR(50),
    resource_id    UUID,
    old_value      JSONB,
    new_value      JSONB,
    ip_address     INET,
    user_agent     TEXT,
    success        BOOLEAN      NOT NULL,
    error_message  TEXT,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- -----------------------------------------------------------------------------
-- Table: password_history
-- Stores previous password hashes to enforce password reuse policy.
-- Decision 2026-02-21: dedicated table (not audit_logs) for efficient lookup.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS password_history (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    password_hash VARCHAR(255) NOT NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- =============================================================================
-- Indexes
-- =============================================================================

-- audit_logs
CREATE INDEX IF NOT EXISTS idx_audit_user_id    ON audit_logs (user_id,        created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_actor_id   ON audit_logs (actor_id,       created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_event_type ON audit_logs (event_type,     created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_app_id     ON audit_logs (application_id, created_at DESC);

-- user_roles
CREATE INDEX IF NOT EXISTS idx_user_roles_user_app ON user_roles (user_id, application_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role      ON user_roles (role_id);

-- user_permissions
CREATE INDEX IF NOT EXISTS idx_user_perms_user_app ON user_permissions (user_id, application_id);

-- user_cost_centers
CREATE INDEX IF NOT EXISTS idx_user_cc_user_app ON user_cost_centers (user_id, application_id);

-- refresh_tokens
CREATE INDEX IF NOT EXISTS idx_refresh_user_app ON refresh_tokens (user_id, app_id);
CREATE INDEX IF NOT EXISTS idx_refresh_expires  ON refresh_tokens (expires_at);

-- permissions
CREATE INDEX IF NOT EXISTS idx_perms_app ON permissions (application_id);

-- roles
CREATE INDEX IF NOT EXISTS idx_roles_app ON roles (application_id);

-- password_history
CREATE INDEX IF NOT EXISTS idx_password_history_user ON password_history (user_id, created_at DESC);

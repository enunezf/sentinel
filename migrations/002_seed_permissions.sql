-- =============================================================================
-- Migration 002: Seed base permissions
-- Project: Sentinel Authentication Service
--
-- Seeds the system permissions required by the admin API and authorization
-- spec into the sentinel application. This migration is idempotent (uses
-- INSERT ... ON CONFLICT DO NOTHING).
--
-- Prerequisites: Migration 001 must be applied first and at least one
-- application with slug = 'sentinel' must exist (created by bootstrap).
-- =============================================================================

DO $$
DECLARE
    v_app_id UUID;
BEGIN
    -- Resolve the sentinel application ID.
    SELECT id INTO v_app_id
    FROM applications
    WHERE slug = 'sentinel';

    IF v_app_id IS NULL THEN
        RAISE NOTICE 'Application "sentinel" not found — skipping permission seed. Run bootstrap first.';
        RETURN;
    END IF;

    -- -------------------------------------------------------------------------
    -- System administration permissions (admin.system.*)
    -- Required by: all /admin/* endpoints
    -- -------------------------------------------------------------------------
    INSERT INTO permissions (application_id, code, description, scope_type)
    VALUES
        (v_app_id, 'admin.system.manage',
         'Full administrative access: users, roles, permissions, cost centers, audit logs',
         'global')
    ON CONFLICT (application_id, code) DO NOTHING;

    -- -------------------------------------------------------------------------
    -- User management permissions (admin.users.*)
    -- -------------------------------------------------------------------------
    INSERT INTO permissions (application_id, code, description, scope_type)
    VALUES
        (v_app_id, 'admin.users.read',
         'View user list and user details', 'resource'),
        (v_app_id, 'admin.users.create',
         'Create new users', 'action'),
        (v_app_id, 'admin.users.update',
         'Update user attributes (username, email, is_active)', 'action'),
        (v_app_id, 'admin.users.unlock',
         'Unlock blocked accounts', 'action'),
        (v_app_id, 'admin.users.reset-password',
         'Force password reset for a user', 'action'),
        (v_app_id, 'admin.users.roles.assign',
         'Assign roles to users', 'action'),
        (v_app_id, 'admin.users.roles.revoke',
         'Revoke roles from users', 'action'),
        (v_app_id, 'admin.users.permissions.assign',
         'Assign individual permissions to users', 'action'),
        (v_app_id, 'admin.users.permissions.revoke',
         'Revoke individual permissions from users', 'action'),
        (v_app_id, 'admin.users.cost-centers.assign',
         'Assign cost centers to users', 'action')
    ON CONFLICT (application_id, code) DO NOTHING;

    -- -------------------------------------------------------------------------
    -- Role management permissions (admin.roles.*)
    -- -------------------------------------------------------------------------
    INSERT INTO permissions (application_id, code, description, scope_type)
    VALUES
        (v_app_id, 'admin.roles.read',
         'View role list and role details', 'resource'),
        (v_app_id, 'admin.roles.create',
         'Create new roles', 'action'),
        (v_app_id, 'admin.roles.update',
         'Update role name and description', 'action'),
        (v_app_id, 'admin.roles.delete',
         'Soft-delete (deactivate) non-system roles', 'action'),
        (v_app_id, 'admin.roles.permissions.assign',
         'Assign permissions to roles', 'action'),
        (v_app_id, 'admin.roles.permissions.revoke',
         'Revoke permissions from roles', 'action')
    ON CONFLICT (application_id, code) DO NOTHING;

    -- -------------------------------------------------------------------------
    -- Permission management permissions (admin.permissions.*)
    -- -------------------------------------------------------------------------
    INSERT INTO permissions (application_id, code, description, scope_type)
    VALUES
        (v_app_id, 'admin.permissions.read',
         'View permission catalogue', 'resource'),
        (v_app_id, 'admin.permissions.create',
         'Create new permissions', 'action'),
        (v_app_id, 'admin.permissions.delete',
         'Delete permissions (cascades to role_permissions and user_permissions)', 'action')
    ON CONFLICT (application_id, code) DO NOTHING;

    -- -------------------------------------------------------------------------
    -- Cost center management permissions (admin.cost-centers.*)
    -- -------------------------------------------------------------------------
    INSERT INTO permissions (application_id, code, description, scope_type)
    VALUES
        (v_app_id, 'admin.cost-centers.read',
         'View cost center list', 'resource'),
        (v_app_id, 'admin.cost-centers.create',
         'Create new cost centers', 'action'),
        (v_app_id, 'admin.cost-centers.update',
         'Update cost center attributes (name, is_active)', 'action')
    ON CONFLICT (application_id, code) DO NOTHING;

    -- -------------------------------------------------------------------------
    -- Audit log permissions (admin.audit.*)
    -- -------------------------------------------------------------------------
    INSERT INTO permissions (application_id, code, description, scope_type)
    VALUES
        (v_app_id, 'admin.audit.read',
         'View audit log entries with filtering', 'resource')
    ON CONFLICT (application_id, code) DO NOTHING;

    -- -------------------------------------------------------------------------
    -- Authorization service permissions (authz.*)
    -- -------------------------------------------------------------------------
    INSERT INTO permissions (application_id, code, description, scope_type)
    VALUES
        (v_app_id, 'authz.verify',
         'Delegated permission verification via POST /authz/verify', 'action'),
        (v_app_id, 'authz.permissions-map.read',
         'Download the signed global permissions map', 'action')
    ON CONFLICT (application_id, code) DO NOTHING;

    RAISE NOTICE 'Seed 002: base permissions inserted for application id = %', v_app_id;
END;
$$;

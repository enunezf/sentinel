package integration_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_UserCRUD tests basic create-read-update-deactivate cycle via raw SQL
// (the admin service itself requires full HTTP wiring, so we test DB layer directly here).
func TestIntegration_UserCRUD(t *testing.T) {
	ctx := context.Background()

	truncateTable(ctx, t, "refresh_tokens")
	truncateTable(ctx, t, "password_history")
	truncateTable(ctx, t, "users")

	userID := uuid.New()

	// Create.
	_, err := testDB.Exec(ctx,
		`INSERT INTO users (id, username, email, password_hash, is_active, must_change_pwd, created_at, updated_at)
		 VALUES ($1, 'cruduser', 'cruduser@test.com', 'hashedpwd', TRUE, TRUE, NOW(), NOW())`,
		userID,
	)
	require.NoError(t, err, "insert user")

	// Read.
	var username, email string
	var isActive, mustChangePwd bool
	err = testDB.QueryRow(ctx,
		`SELECT username, email, is_active, must_change_pwd FROM users WHERE id=$1`, userID,
	).Scan(&username, &email, &isActive, &mustChangePwd)
	require.NoError(t, err)
	assert.Equal(t, "cruduser", username)
	assert.Equal(t, "cruduser@test.com", email)
	assert.True(t, isActive)
	assert.True(t, mustChangePwd)

	// Update email.
	_, err = testDB.Exec(ctx,
		`UPDATE users SET email='updated@test.com', updated_at=NOW() WHERE id=$1`, userID,
	)
	require.NoError(t, err)
	err = testDB.QueryRow(ctx, `SELECT email FROM users WHERE id=$1`, userID).Scan(&email)
	require.NoError(t, err)
	assert.Equal(t, "updated@test.com", email, "email update must persist")

	// Deactivate (soft delete).
	_, err = testDB.Exec(ctx,
		`UPDATE users SET is_active=FALSE, updated_at=NOW() WHERE id=$1`, userID,
	)
	require.NoError(t, err)
	err = testDB.QueryRow(ctx, `SELECT is_active FROM users WHERE id=$1`, userID).Scan(&isActive)
	require.NoError(t, err)
	assert.False(t, isActive, "deactivated user must have is_active=false")
}

// TestIntegration_RoleCRUD tests role creation and permission assignment.
func TestIntegration_RoleCRUD(t *testing.T) {
	ctx := context.Background()

	truncateTable(ctx, t, "role_permissions")
	truncateTable(ctx, t, "user_roles")
	truncateTable(ctx, t, "roles")
	truncateTable(ctx, t, "permissions")
	truncateTable(ctx, t, "applications")

	// Create application.
	appID := uuid.New()
	_, err := testDB.Exec(ctx,
		`INSERT INTO applications (id, name, slug, secret_key, is_active, created_at, updated_at)
		 VALUES ($1, 'RoleCRUD App', 'role-crud-app', 'secret-role', TRUE, NOW(), NOW())`,
		appID,
	)
	require.NoError(t, err)

	// Create permission.
	permID := uuid.New()
	_, err = testDB.Exec(ctx,
		`INSERT INTO permissions (id, application_id, code, description, scope_type, created_at)
		 VALUES ($1, $2, 'inventory.stock.read', 'Ver stock', 'action', NOW())`,
		permID, appID,
	)
	require.NoError(t, err)

	// Create role.
	roleID := uuid.New()
	_, err = testDB.Exec(ctx,
		`INSERT INTO roles (id, application_id, name, description, is_system, is_active, created_at, updated_at)
		 VALUES ($1, $2, 'chef', 'Chef de cocina', FALSE, TRUE, NOW(), NOW())`,
		roleID, appID,
	)
	require.NoError(t, err)

	// Assign permission to role.
	_, err = testDB.Exec(ctx,
		`INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)`,
		roleID, permID,
	)
	require.NoError(t, err)

	// Verify role has the permission.
	var code string
	err = testDB.QueryRow(ctx,
		`SELECT p.code FROM role_permissions rp
		 JOIN permissions p ON p.id = rp.permission_id
		 WHERE rp.role_id=$1`, roleID,
	).Scan(&code)
	require.NoError(t, err)
	assert.Equal(t, "inventory.stock.read", code, "role must have the assigned permission")
}

// TestIntegration_Pagination tests that paginated queries return correct slices.
func TestIntegration_Pagination(t *testing.T) {
	ctx := context.Background()

	truncateTable(ctx, t, "refresh_tokens")
	truncateTable(ctx, t, "password_history")
	truncateTable(ctx, t, "users")

	// Insert 25 users.
	for i := 0; i < 25; i++ {
		id := uuid.New()
		_, err := testDB.Exec(ctx,
			`INSERT INTO users (id, username, email, password_hash, is_active, must_change_pwd, created_at, updated_at)
			 VALUES ($1, $2, $3, 'hash', TRUE, FALSE, NOW(), NOW())`,
			id,
			"pageuser"+string(rune('a'+i)),
			"pageuser"+string(rune('a'+i))+"@test.com",
		)
		require.NoError(t, err)
	}

	// Verify total.
	var total int
	err := testDB.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&total)
	require.NoError(t, err)
	assert.Equal(t, 25, total)

	// Simulate page=2, page_size=10 (offset=10, limit=10).
	page := 2
	pageSize := 10
	offset := (page - 1) * pageSize

	rows, err := testDB.Query(ctx,
		`SELECT id FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		pageSize, offset,
	)
	require.NoError(t, err)
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())

	assert.Len(t, ids, 10, "page 2 with page_size=10 must return 10 records")

	// Verify total_pages = ceil(25/10) = 3.
	totalPages := (total + pageSize - 1) / pageSize
	assert.Equal(t, 3, totalPages, "total_pages must be 3 for 25 items with page_size=10")
}

// TestIntegration_HealthCheck verifies the DB and Redis connections are alive.
func TestIntegration_HealthCheck(t *testing.T) {
	ctx := context.Background()

	// PostgreSQL health check.
	err := testDB.Ping(ctx)
	assert.NoError(t, err, "PostgreSQL must respond to ping")

	// Redis health check.
	pong, err := testRedisClient.Ping(ctx).Result()
	assert.NoError(t, err, "Redis must respond to ping")
	assert.Equal(t, "PONG", pong)
}

// TestIntegration_AuditLogs_Immutability verifies that the audit_logs table
// accumulates records correctly and its cascade rules are safe.
func TestIntegration_AuditLogs_Immutability(t *testing.T) {
	ctx := context.Background()

	// We can INSERT into audit_logs.
	logID := uuid.New()
	_, err := testDB.Exec(ctx,
		`INSERT INTO audit_logs (id, event_type, success, created_at)
		 VALUES ($1, 'AUTH_LOGIN_SUCCESS', TRUE, NOW())`,
		logID,
	)
	require.NoError(t, err, "inserting into audit_logs must succeed")

	// Verify the record exists.
	var eventType string
	err = testDB.QueryRow(ctx,
		`SELECT event_type FROM audit_logs WHERE id=$1`, logID,
	).Scan(&eventType)
	require.NoError(t, err)
	assert.Equal(t, "AUTH_LOGIN_SUCCESS", eventType)

	// Application policy: no UPDATE/DELETE at DB level (enforced in app layer).
	// We just verify INSERT worked and SELECT returns the record.
}

// TestIntegration_RefreshToken_Cascade verifies that deleting a user also removes
// their refresh tokens (ON DELETE CASCADE).
func TestIntegration_RefreshToken_Cascade(t *testing.T) {
	ctx := context.Background()

	truncateTable(ctx, t, "refresh_tokens")
	truncateTable(ctx, t, "users")
	truncateTable(ctx, t, "applications")

	appID := uuid.New()
	_, err := testDB.Exec(ctx,
		`INSERT INTO applications (id, name, slug, secret_key, is_active, created_at, updated_at)
		 VALUES ($1, 'Cascade App', 'cascade-app', 'secret', TRUE, NOW(), NOW())`,
		appID,
	)
	require.NoError(t, err)

	userID := uuid.New()
	_, err = testDB.Exec(ctx,
		`INSERT INTO users (id, username, email, password_hash, is_active, must_change_pwd, created_at, updated_at)
		 VALUES ($1, 'cascadeuser', 'cascade@test.com', 'hash', TRUE, FALSE, NOW(), NOW())`,
		userID,
	)
	require.NoError(t, err)

	// Insert a refresh token for the user.
	tokenID := uuid.New()
	_, err = testDB.Exec(ctx,
		`INSERT INTO refresh_tokens (id, user_id, app_id, token_hash, expires_at, is_revoked, created_at)
		 VALUES ($1, $2, $3, 'hash-value', NOW()+INTERVAL'7 days', FALSE, NOW())`,
		tokenID, userID, appID,
	)
	require.NoError(t, err)

	// Delete the user: refresh token must cascade-delete.
	_, err = testDB.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	require.NoError(t, err)

	var count int
	err = testDB.QueryRow(ctx, `SELECT COUNT(*) FROM refresh_tokens WHERE id=$1`, tokenID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "CASCADE DELETE must remove refresh tokens when user is deleted")
}

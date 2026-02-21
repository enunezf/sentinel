package integration_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/enunezf/sentinel/internal/config"
	"github.com/enunezf/sentinel/internal/domain"
	pgrepository "github.com/enunezf/sentinel/internal/repository/postgres"
	redisrepository "github.com/enunezf/sentinel/internal/repository/redis"
	"github.com/enunezf/sentinel/internal/service"
	"github.com/enunezf/sentinel/internal/token"
)

// makeAuthSvc creates a fully wired AuthServiceI with real repos connecting to test containers.
func makeAuthSvc(t *testing.T) *service.AuthServiceI {
	t.Helper()

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	mgr := token.NewManagerFromKey(privKey)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			AccessTokenTTL:        60 * time.Minute,
			RefreshTokenTTLWeb:    168 * time.Hour,
			RefreshTokenTTLMobile: 720 * time.Hour,
		},
		Security: config.SecurityConfig{
			MaxFailedAttempts: 5,
			LockoutDuration:   15 * time.Minute,
			BcryptCost:        4, // Low cost for speed in tests.
			PasswordHistory:   5,
		},
	}

	userRepo := pgrepository.NewUserRepository(testDB)
	appRepo := pgrepository.NewApplicationRepository(testDB)
	pgRefresh := pgrepository.NewRefreshTokenRepository(testDB)
	redisRefresh := redisrepository.NewRefreshTokenRepository(testRedisClient)
	pwdHistory := pgrepository.NewPasswordHistoryRepository(testDB)
	userRoleRepo := pgrepository.NewUserRoleRepository(testDB)

	// Null audit service (logs to /dev/null for integration tests).
	auditRepo := pgrepository.NewAuditRepository(testDB)
	auditSvc := service.NewAuditService(auditRepo)
	t.Cleanup(func() { auditSvc.Close() })

	return service.NewAuthServiceI(
		userRepo, appRepo, pgRefresh, redisRefresh, pwdHistory, userRoleRepo,
		mgr, auditSvc, cfg,
	)
}

// insertTestApp inserts an application into the test DB and returns it.
func insertTestApp(t *testing.T, ctx context.Context, slug, secretKey string) *domain.Application {
	t.Helper()
	app := &domain.Application{
		ID:        uuid.New(),
		Name:      "Integration Test App " + slug,
		Slug:      slug,
		SecretKey: secretKey,
		IsActive:  true,
	}
	_, err := testDB.Exec(ctx,
		`INSERT INTO applications (id, name, slug, secret_key, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`,
		app.ID, app.Name, app.Slug, app.SecretKey, app.IsActive,
	)
	require.NoError(t, err, "insert test application")
	return app
}

// insertTestUser inserts a user with the given plain password (auto-hashed) and returns it.
func insertTestUser(t *testing.T, ctx context.Context, username, email, plainPassword string) *domain.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), 4)
	require.NoError(t, err)

	user := &domain.User{
		ID:            uuid.New(),
		Username:      username,
		Email:         email,
		PasswordHash:  string(hash),
		IsActive:      true,
		MustChangePwd: false,
	}
	_, err = testDB.Exec(ctx,
		`INSERT INTO users (id, username, email, password_hash, is_active, must_change_pwd, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
		user.ID, user.Username, user.Email, user.PasswordHash, user.IsActive, user.MustChangePwd,
	)
	require.NoError(t, err, "insert test user")
	return user
}

// TestIntegration_Login_FullFlow tests the complete login -> refresh -> logout -> revoked cycle.
func TestIntegration_Login_FullFlow(t *testing.T) {
	ctx := context.Background()

	// Setup.
	truncateTable(ctx, t, "refresh_tokens")
	truncateTable(ctx, t, "users")
	truncateTable(ctx, t, "applications")

	app := insertTestApp(t, ctx, "test-app-flow", "secret-flow-key")
	user := insertTestUser(t, ctx, "flowuser", "flowuser@test.com", "S3cur3P@ss!")
	svc := makeAuthSvc(t)

	// Step 1: Login.
	loginResp, err := svc.Login(ctx, service.LoginRequest{
		Username:   user.Username,
		Password:   "S3cur3P@ss!",
		ClientType: "web",
		AppKey:     app.SecretKey,
	})
	require.NoError(t, err, "login must succeed")
	assert.NotEmpty(t, loginResp.AccessToken)
	assert.NotEmpty(t, loginResp.RefreshToken)
	assert.Equal(t, "Bearer", loginResp.TokenType)
	assert.Equal(t, 3600, loginResp.ExpiresIn)

	// Step 2: Refresh.
	refreshResp, err := svc.Refresh(ctx, service.RefreshRequest{
		RefreshToken: loginResp.RefreshToken,
		AppKey:       app.SecretKey,
	})
	require.NoError(t, err, "refresh must succeed")
	assert.NotEmpty(t, refreshResp.AccessToken)
	assert.NotEmpty(t, refreshResp.RefreshToken)
	assert.NotEqual(t, loginResp.RefreshToken, refreshResp.RefreshToken, "token rotation must produce new token")

	// Step 3: Logout.
	claims := &domain.Claims{Sub: user.ID.String(), App: app.Slug}
	err = svc.Logout(ctx, claims, app.SecretKey, "127.0.0.1", "test")
	require.NoError(t, err, "logout must succeed")

	// Step 4: Refresh with revoked token must fail.
	_, err = svc.Refresh(ctx, service.RefreshRequest{
		RefreshToken: refreshResp.RefreshToken,
		AppKey:       app.SecretKey,
	})
	assert.ErrorIs(t, err, service.ErrTokenRevoked, "revoked token must be rejected")
}

// TestIntegration_BruteForce tests that 5 failed attempts locks the account.
func TestIntegration_BruteForce(t *testing.T) {
	ctx := context.Background()

	truncateTable(ctx, t, "refresh_tokens")
	truncateTable(ctx, t, "users")
	truncateTable(ctx, t, "applications")

	app := insertTestApp(t, ctx, "test-app-bf", "secret-bf-key")
	user := insertTestUser(t, ctx, "bfuser", "bfuser@test.com", "S3cur3P@ss!")
	svc := makeAuthSvc(t)

	// 4 failed attempts must NOT lock.
	for i := 0; i < 4; i++ {
		_, err := svc.Login(ctx, service.LoginRequest{
			Username:   user.Username,
			Password:   "WRONG_PWD",
			ClientType: "web",
			AppKey:     app.SecretKey,
		})
		assert.ErrorIs(t, err, service.ErrInvalidCredentials, "attempt %d must return INVALID_CREDENTIALS", i+1)
	}

	// Verify failed_attempts in DB = 4.
	var attempts int
	err := testDB.QueryRow(ctx, `SELECT failed_attempts FROM users WHERE id = $1`, user.ID).Scan(&attempts)
	require.NoError(t, err)
	assert.Equal(t, 4, attempts, "4 failures must set failed_attempts=4")

	// 5th attempt must lock.
	_, err = svc.Login(ctx, service.LoginRequest{
		Username:   user.Username,
		Password:   "WRONG_PWD",
		ClientType: "web",
		AppKey:     app.SecretKey,
	})
	assert.ErrorIs(t, err, service.ErrInvalidCredentials)

	// Verify DB: failed_attempts=5, locked_until set.
	var failedAttempts int
	var lockedUntil *time.Time
	err = testDB.QueryRow(ctx,
		`SELECT failed_attempts, locked_until FROM users WHERE id = $1`, user.ID,
	).Scan(&failedAttempts, &lockedUntil)
	require.NoError(t, err)
	assert.Equal(t, 5, failedAttempts, "failed_attempts must be 5 after 5th failure")
	require.NotNil(t, lockedUntil, "locked_until must be set after 5th failure")
	assert.True(t, lockedUntil.After(time.Now()), "locked_until must be in the future")

	// 6th attempt with correct password must return ACCOUNT_LOCKED.
	_, err = svc.Login(ctx, service.LoginRequest{
		Username:   user.Username,
		Password:   "S3cur3P@ss!",
		ClientType: "web",
		AppKey:     app.SecretKey,
	})
	assert.ErrorIs(t, err, service.ErrAccountLocked, "locked account must return ACCOUNT_LOCKED")
}

// TestIntegration_PermanentLock simulates 3 lockouts on the same day => permanent lock.
func TestIntegration_PermanentLock(t *testing.T) {
	ctx := context.Background()

	truncateTable(ctx, t, "refresh_tokens")
	truncateTable(ctx, t, "users")
	truncateTable(ctx, t, "applications")

	app := insertTestApp(t, ctx, "test-app-perm", "secret-perm-key")
	user := insertTestUser(t, ctx, "permuser", "permuser@test.com", "S3cur3P@ss!")
	svc := makeAuthSvc(t)

	// Pre-seed: lockout_count=2 and lockout_date=TODAY to simulate 2 prior lockouts today.
	today := time.Now().UTC().Truncate(24 * time.Hour)
	_, err := testDB.Exec(ctx,
		`UPDATE users SET lockout_count=$2, lockout_date=$3 WHERE id=$1`,
		user.ID, 2, today,
	)
	require.NoError(t, err)

	// Trigger 5 failures to reach the 3rd lockout.
	for i := 0; i < 5; i++ {
		_, _ = svc.Login(ctx, service.LoginRequest{
			Username:   user.Username,
			Password:   "WRONG",
			ClientType: "web",
			AppKey:     app.SecretKey,
		})
	}

	// Verify: locked_until must be NULL (permanent), lockout_count >= 3.
	var lockedUntil *time.Time
	var lockoutCount int
	err = testDB.QueryRow(ctx,
		`SELECT locked_until, lockout_count FROM users WHERE id=$1`, user.ID,
	).Scan(&lockedUntil, &lockoutCount)
	require.NoError(t, err)
	assert.Nil(t, lockedUntil, "permanent lock must have locked_until=NULL")
	assert.GreaterOrEqual(t, lockoutCount, 3, "lockout_count must be >= 3 for permanent lock")
}

// TestIntegration_ChangePassword_History tests password reuse detection.
func TestIntegration_ChangePassword_History(t *testing.T) {
	ctx := context.Background()

	truncateTable(ctx, t, "password_history")
	truncateTable(ctx, t, "refresh_tokens")
	truncateTable(ctx, t, "users")
	truncateTable(ctx, t, "applications")

	insertTestApp(t, ctx, "test-app-hist", "secret-hist-key")
	user := insertTestUser(t, ctx, "histuser", "histuser@test.com", "Password1!a")
	svc := makeAuthSvc(t)
	claims := &domain.Claims{Sub: user.ID.String()}

	// Change password 6 times so that the very first password (Password1!a) falls
	// outside the last-5 window (it becomes the 6th oldest entry).
	passwords := []string{
		"Password1!a", // index 0: creation password (not in history yet)
		"Password2!b",
		"Password3!c",
		"Password4!d",
		"Password5!e",
		"Password6!f",
		"Password7!g",
	}
	for i := 0; i < 6; i++ {
		err := svc.ChangePassword(ctx, claims, service.ChangePasswordRequest{
			CurrentPassword: passwords[i],
			NewPassword:     passwords[i+1],
		})
		require.NoError(t, err, "change to password %d must succeed", i+1)
	}
	// History (most recent 5, after 6 changes):
	//   1. Password6!f  (was current before last change)
	//   2. Password5!e
	//   3. Password4!d
	//   4. Password3!c
	//   5. Password2!b
	// Password1!a is the 6th entry — outside the last-5 window.

	// Attempt to reuse "Password3!c" (in last 5) -> must fail.
	err := svc.ChangePassword(ctx, claims, service.ChangePasswordRequest{
		CurrentPassword: "Password7!g",
		NewPassword:     "Password3!c",
	})
	assert.ErrorIs(t, err, service.ErrPasswordReused, "reusing a password in last 5 must be rejected")

	// Attempt to use "Password1!a" (now 6th old, outside last-5) -> must succeed.
	err = svc.ChangePassword(ctx, claims, service.ChangePasswordRequest{
		CurrentPassword: "Password7!g",
		NewPassword:     "Password1!a",
	})
	assert.NoError(t, err, "password older than last 5 must be accepted")
}

// TestIntegration_Bootstrap verifies that the system bootstrap is idempotent.
func TestIntegration_Bootstrap(t *testing.T) {
	ctx := context.Background()

	// We test the bootstrap invariant by checking:
	// 1. Fresh DB can accept a new application + user.
	// 2. Running the same inserts again with ON CONFLICT DO NOTHING doesn't duplicate.

	// Insert the "system" application (simulating bootstrap).
	systemAppID := uuid.New()
	_, err := testDB.Exec(ctx,
		`INSERT INTO applications (id, name, slug, secret_key, is_active, created_at, updated_at)
		 VALUES ($1, 'System', 'system', 'system-secret', TRUE, NOW(), NOW())
		 ON CONFLICT (slug) DO NOTHING`,
		systemAppID,
	)
	require.NoError(t, err)

	// Second run must be idempotent.
	_, err = testDB.Exec(ctx,
		`INSERT INTO applications (id, name, slug, secret_key, is_active, created_at, updated_at)
		 VALUES ($1, 'System', 'system', 'system-secret', TRUE, NOW(), NOW())
		 ON CONFLICT (slug) DO NOTHING`,
		uuid.New(),
	)
	require.NoError(t, err, "second bootstrap insert must not fail (ON CONFLICT DO NOTHING)")

	// Verify only one 'system' application exists.
	var count int
	err = testDB.QueryRow(ctx, `SELECT COUNT(*) FROM applications WHERE slug='system'`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "bootstrap must not create duplicate system application")
}

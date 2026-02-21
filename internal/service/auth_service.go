package service

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/unicode/norm"

	"github.com/enunezf/sentinel/internal/config"
	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
	redisrepo "github.com/enunezf/sentinel/internal/repository/redis"
	"github.com/enunezf/sentinel/internal/token"
)

// Sentinel error codes for auth operations.
var (
	ErrInvalidCredentials  = errors.New("INVALID_CREDENTIALS")
	ErrAccountInactive     = errors.New("ACCOUNT_INACTIVE")
	ErrAccountLocked       = errors.New("ACCOUNT_LOCKED")
	ErrTokenInvalid        = errors.New("TOKEN_INVALID")
	ErrTokenExpired        = errors.New("TOKEN_EXPIRED")
	ErrTokenRevoked        = errors.New("TOKEN_REVOKED")
	ErrPasswordPolicy      = errors.New("VALIDATION_ERROR")
	ErrPasswordReused      = errors.New("PASSWORD_REUSED")
	ErrApplicationNotFound = errors.New("APPLICATION_NOT_FOUND")
	ErrInvalidClientType   = errors.New("INVALID_CLIENT_TYPE")
)

// AuthService implements authentication business logic.
type AuthService struct {
	userRepo         *postgres.UserRepository
	appRepo          *postgres.ApplicationRepository
	refreshPGRepo    *postgres.RefreshTokenRepository
	refreshRedisRepo *redisrepo.RefreshTokenRepository
	pwdHistoryRepo   *postgres.PasswordHistoryRepository
	userRoleRepo     *postgres.UserRoleRepository
	tokenMgr         *token.Manager
	auditSvc         *AuditService
	cfg              *config.Config
}

// NewAuthService creates an AuthService with all required dependencies.
func NewAuthService(
	userRepo *postgres.UserRepository,
	appRepo *postgres.ApplicationRepository,
	refreshPGRepo *postgres.RefreshTokenRepository,
	refreshRedisRepo *redisrepo.RefreshTokenRepository,
	pwdHistoryRepo *postgres.PasswordHistoryRepository,
	userRoleRepo *postgres.UserRoleRepository,
	tokenMgr *token.Manager,
	auditSvc *AuditService,
	cfg *config.Config,
) *AuthService {
	return &AuthService{
		userRepo:         userRepo,
		appRepo:          appRepo,
		refreshPGRepo:    refreshPGRepo,
		refreshRedisRepo: refreshRedisRepo,
		pwdHistoryRepo:   pwdHistoryRepo,
		userRoleRepo:     userRoleRepo,
		tokenMgr:         tokenMgr,
		auditSvc:         auditSvc,
		cfg:              cfg,
	}
}

// LoginRequest holds the input for login.
type LoginRequest struct {
	Username   string
	Password   string
	ClientType string
	AppKey     string
	IP         string
	UserAgent  string
}

// LoginResponse holds the output of a successful login.
type LoginResponse struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int
	User         *domain.User
}

// Login validates credentials and returns tokens on success.
func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	// Step 1: Validate X-App-Key.
	app, err := s.appRepo.FindBySecretKey(ctx, req.AppKey)
	if err != nil {
		return nil, fmt.Errorf("auth: find app: %w", err)
	}
	if app == nil || !app.IsActive {
		return nil, ErrApplicationNotFound
	}

	// Step 1b: Validate client_type.
	if !domain.IsValidClientType(req.ClientType) {
		return nil, ErrInvalidClientType
	}

	// Step 2: Find user by username.
	user, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("auth: find user: %w", err)
	}

	// Step 3: User not found -> INVALID_CREDENTIALS.
	if user == nil {
		appID := app.ID
		s.logAuditFailed(domain.EventAuthLoginFailed, nil, &appID, req.IP, req.UserAgent, "user not found")
		return nil, ErrInvalidCredentials
	}

	// Step 4: Account inactive.
	if !user.IsActive {
		return nil, ErrAccountInactive
	}

	// Step 5: Account locked.
	now := time.Now()
	if user.IsLocked(now) {
		return nil, ErrAccountLocked
	}

	// Step 6: Compare password (NFC-normalized).
	normalizedPwd := norm.NFC.String(req.Password)
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(normalizedPwd)); err != nil {
		// Step 7: Password failed.
		s.handleFailedLogin(ctx, user, app.ID, req.IP, req.UserAgent)
		return nil, ErrInvalidCredentials
	}

	// Step 8: Login successful.
	if err := s.userRepo.UpdateLastLogin(ctx, user.ID); err != nil {
		return nil, fmt.Errorf("auth: update last login: %w", err)
	}

	// Determine TTL based on client_type.
	refreshTTL := s.cfg.JWT.RefreshTokenTTLWeb
	if req.ClientType == string(domain.ClientTypeMobile) || req.ClientType == string(domain.ClientTypeDesktop) {
		refreshTTL = s.cfg.JWT.RefreshTokenTTLMobile
	}

	// Get active roles for the app.
	roles, err := s.userRoleRepo.GetActiveRoleNamesForUserApp(ctx, user.ID, app.ID)
	if err != nil {
		return nil, fmt.Errorf("auth: get roles: %w", err)
	}

	// Generate access token.
	accessToken, err := s.tokenMgr.GenerateAccessToken(user, app.Slug, roles, s.cfg.JWT.AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("auth: generate access token: %w", err)
	}

	// Generate refresh token (UUID v4).
	rawRefreshToken := uuid.New().String()

	// Store refresh token.
	if err := s.storeRefreshToken(ctx, user.ID, app.ID, rawRefreshToken, req.ClientType, req.IP, req.UserAgent, refreshTTL); err != nil {
		return nil, fmt.Errorf("auth: store refresh token: %w", err)
	}

	userID := user.ID
	appID := app.ID
	resType := "user"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:     domain.EventAuthLoginSuccess,
		ApplicationID: &appID,
		UserID:        &userID,
		ActorID:       &userID,
		ResourceType:  &resType,
		ResourceID:    &userID,
		IPAddress:     req.IP,
		UserAgent:     req.UserAgent,
		Success:       true,
	})

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.cfg.JWT.AccessTokenTTL.Seconds()),
		User:         user,
	}, nil
}

// handleFailedLogin increments failed attempts and applies lockout logic per spec.
func (s *AuthService) handleFailedLogin(ctx context.Context, user *domain.User, appID uuid.UUID, ip, ua string) {
	user.FailedAttempts++
	var lockedUntil *time.Time
	lockoutCount := user.LockoutCount
	lockoutDate := user.LockoutDate
	now := time.Now()

	if user.FailedAttempts >= s.cfg.Security.MaxFailedAttempts {
		today := now.UTC().Truncate(24 * time.Hour)
		if lockoutDate == nil || !lockoutDate.UTC().Truncate(24*time.Hour).Equal(today) {
			lockoutCount = 0
			lockoutDate = &today
		}
		lockoutCount++

		if lockoutCount >= 3 {
			// Permanent lock: locked_until = NULL with lockout_count >= 3.
			lockedUntil = nil
		} else {
			t := now.Add(s.cfg.Security.LockoutDuration)
			lockedUntil = &t
		}

		userID := user.ID
		resType := "user"
		s.auditSvc.LogEvent(&domain.AuditLog{
			EventType:     domain.EventAuthAccountLocked,
			ApplicationID: &appID,
			UserID:        &userID,
			ResourceType:  &resType,
			ResourceID:    &userID,
			IPAddress:     ip,
			UserAgent:     ua,
			Success:       true,
		})
	}

	_ = s.userRepo.UpdateFailedAttempts(ctx, user.ID, user.FailedAttempts, lockedUntil, lockoutCount, lockoutDate)

	userID := user.ID
	resType := "user"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:     domain.EventAuthLoginFailed,
		ApplicationID: &appID,
		UserID:        &userID,
		ResourceType:  &resType,
		ResourceID:    &userID,
		NewValue:      map[string]interface{}{"failed_attempts": user.FailedAttempts},
		IPAddress:     ip,
		UserAgent:     ua,
		Success:       false,
		ErrorMessage:  "Invalid credentials",
	})
}

func (s *AuthService) logAuditFailed(et domain.EventType, userID *uuid.UUID, appID *uuid.UUID, ip, ua, msg string) {
	resType := "user"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:     et,
		ApplicationID: appID,
		UserID:        userID,
		ResourceType:  &resType,
		IPAddress:     ip,
		UserAgent:     ua,
		Success:       false,
		ErrorMessage:  msg,
	})
}

// storeRefreshToken hashes and stores a refresh token.
// Design decision: The raw UUID v4 token is used as the Redis lookup key
// (key: "refresh:<rawToken>"). The bcrypt hash is stored in PostgreSQL as
// token_hash (UNIQUE). On lookup, we first check Redis with the raw token
// to get the bcrypt hash, then look up in PG by hash. This satisfies the spec
// (PG stores bcrypt hash) while enabling O(1) Redis lookup without requiring
// deterministic hashing of the token.
func (s *AuthService) storeRefreshToken(ctx context.Context, userID, appID uuid.UUID, rawToken, clientType, ip, ua string, ttl time.Duration) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(rawToken), s.cfg.Security.BcryptCost)
	if err != nil {
		return fmt.Errorf("auth: hash refresh token: %w", err)
	}
	hashStr := string(hash)

	rt := &domain.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		AppID:     appID,
		TokenHash: hashStr,
		DeviceInfo: domain.DeviceInfo{
			UserAgent:  ua,
			IP:         ip,
			ClientType: clientType,
		},
		ExpiresAt: time.Now().Add(ttl),
	}

	if err := s.refreshPGRepo.Create(ctx, rt); err != nil {
		return fmt.Errorf("auth: pg create refresh token: %w", err)
	}

	// Redis key is the raw token (UUID v4) for O(1) lookup.
	// Value includes the bcrypt hash so we can look up the PG record by hash.
	redisData := redisrepo.RefreshTokenData{
		UserID:     userID.String(),
		AppID:      appID.String(),
		ExpiresAt:  rt.ExpiresAt.Format(time.RFC3339),
		ClientType: clientType,
		UserAgent:  ua,
		IP:         ip,
		TokenHash:  hashStr,
	}
	if err := s.refreshRedisRepo.Set(ctx, rawToken, redisData, ttl); err != nil {
		// Non-fatal: PostgreSQL is source of truth.
		_ = err
	}
	return nil
}

// findRefreshTokenByRaw finds a refresh token by the raw UUID value.
// Redis key: "refresh:<rawToken>" -> gets metadata.
// PG lookup: we need to find the PG record by the raw token.
// Since PG stores bcrypt hash (non-deterministic), we add a raw_token_id column
// OR we store the bcrypt hash in Redis and look up PG by hash.
// We resolve this by storing the bcrypt hash in the Redis value.
func (s *AuthService) findRefreshTokenByRaw(ctx context.Context, rawToken string) (*domain.RefreshToken, error) {
	// Look up in Redis first: key = "refresh:<rawToken>".
	// The Redis value stores metadata but NOT the bcrypt hash.
	// We need the PG record. Since token_hash is UNIQUE, we need a way to find it.
	//
	// Resolution: We store an additional field "token_hash" in the Redis value.
	// Let's update the Redis data struct to include the bcrypt hash.
	data, err := s.refreshRedisRepo.Get(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("auth: redis get refresh token: %w", err)
	}

	if data != nil && data.TokenHash != "" {
		// Redis hit: look up PG by hash.
		rt, err := s.refreshPGRepo.FindByHash(ctx, data.TokenHash)
		if err != nil {
			return nil, fmt.Errorf("auth: pg find refresh by hash: %w", err)
		}
		return rt, nil
	}

	// Redis miss: fall back to PG full scan with bcrypt comparison.
	// This is O(n) but acceptable for ~2000 users at v1 scale.
	// For production scale, a SHA-256 lookup column should be added.
	return s.refreshPGRepo.FindByRawToken(ctx, rawToken)
}

// RefreshRequest holds input for token refresh.
type RefreshRequest struct {
	RefreshToken string
	AppKey       string
	IP           string
	UserAgent    string
}

// RefreshResponse holds output for token refresh.
type RefreshResponse struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int
}

// Refresh validates a refresh token and issues new tokens (rotation).
func (s *AuthService) Refresh(ctx context.Context, req RefreshRequest) (*RefreshResponse, error) {
	// Validate App Key.
	app, err := s.appRepo.FindBySecretKey(ctx, req.AppKey)
	if err != nil || app == nil || !app.IsActive {
		return nil, ErrApplicationNotFound
	}

	rtRecord, err := s.findRefreshTokenByRaw(ctx, req.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("auth: find refresh token: %w", err)
	}
	if rtRecord == nil {
		return nil, ErrTokenInvalid
	}
	if rtRecord.IsRevoked {
		return nil, ErrTokenRevoked
	}
	if time.Now().After(rtRecord.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	// Verify user is still active and not locked.
	user, err := s.userRepo.FindByID(ctx, rtRecord.UserID)
	if err != nil || user == nil {
		return nil, ErrInvalidCredentials
	}
	if !user.IsActive {
		return nil, ErrAccountInactive
	}
	if user.IsLocked(time.Now()) {
		return nil, ErrAccountLocked
	}

	// Revoke current token (both PG and Redis).
	if err := s.refreshPGRepo.Revoke(ctx, rtRecord.ID); err != nil {
		return nil, fmt.Errorf("auth: revoke refresh token: %w", err)
	}
	_ = s.refreshRedisRepo.Delete(ctx, req.RefreshToken)

	// Get roles for the app.
	roles, err := s.userRoleRepo.GetActiveRoleNamesForUserApp(ctx, user.ID, rtRecord.AppID)
	if err != nil {
		return nil, fmt.Errorf("auth: get roles: %w", err)
	}

	// Get app slug.
	appObj, err := s.appRepo.FindBySecretKey(ctx, req.AppKey)
	if err != nil || appObj == nil {
		return nil, ErrApplicationNotFound
	}

	// Generate new tokens.
	accessToken, err := s.tokenMgr.GenerateAccessToken(user, appObj.Slug, roles, s.cfg.JWT.AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("auth: generate access token: %w", err)
	}

	rawRefreshToken := uuid.New().String()
	ttl := s.cfg.JWT.RefreshTokenTTLWeb
	if rtRecord.DeviceInfo.ClientType == string(domain.ClientTypeMobile) ||
		rtRecord.DeviceInfo.ClientType == string(domain.ClientTypeDesktop) {
		ttl = s.cfg.JWT.RefreshTokenTTLMobile
	}

	if err := s.storeRefreshToken(ctx, user.ID, rtRecord.AppID, rawRefreshToken, rtRecord.DeviceInfo.ClientType, req.IP, req.UserAgent, ttl); err != nil {
		return nil, fmt.Errorf("auth: store new refresh token: %w", err)
	}

	userID := user.ID
	appID := rtRecord.AppID
	resType := "refresh_token"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:     domain.EventAuthTokenRefreshed,
		ApplicationID: &appID,
		UserID:        &userID,
		ActorID:       &userID,
		ResourceType:  &resType,
		IPAddress:     req.IP,
		UserAgent:     req.UserAgent,
		Success:       true,
	})

	return &RefreshResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.cfg.JWT.AccessTokenTTL.Seconds()),
	}, nil
}

// Logout revokes all active refresh tokens for the user+app.
func (s *AuthService) Logout(ctx context.Context, claims *domain.Claims, appKey, ip, ua string) error {
	app, err := s.appRepo.FindBySecretKey(ctx, appKey)
	if err != nil || app == nil || !app.IsActive {
		return ErrApplicationNotFound
	}

	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return ErrInvalidCredentials
	}

	if err := s.refreshPGRepo.RevokeAllForUser(ctx, userID, app.ID); err != nil {
		return fmt.Errorf("auth: revoke all refresh tokens: %w", err)
	}

	appID := app.ID
	resType := "refresh_token"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:     domain.EventAuthLogout,
		ApplicationID: &appID,
		UserID:        &userID,
		ActorID:       &userID,
		ResourceType:  &resType,
		IPAddress:     ip,
		UserAgent:     ua,
		Success:       true,
	})
	return nil
}

// ChangePasswordRequest holds input for change-password.
type ChangePasswordRequest struct {
	CurrentPassword string
	NewPassword     string
	IP              string
	UserAgent       string
}

// ChangePassword validates the current password and sets a new one.
func (s *AuthService) ChangePassword(ctx context.Context, claims *domain.Claims, req ChangePasswordRequest) error {
	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return ErrInvalidCredentials
	}

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil || user == nil {
		return ErrInvalidCredentials
	}

	// Normalize NFC.
	currentNFC := norm.NFC.String(req.CurrentPassword)
	newNFC := norm.NFC.String(req.NewPassword)

	// Verify current password.
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentNFC)); err != nil {
		return ErrInvalidCredentials
	}

	// Validate password policy.
	if err := ValidatePasswordPolicy(newNFC); err != nil {
		return err
	}

	// Check password history (last 5).
	hashes, err := s.pwdHistoryRepo.GetLastN(ctx, userID, s.cfg.Security.PasswordHistory)
	if err != nil {
		return fmt.Errorf("auth: get password history: %w", err)
	}
	for _, h := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(h), []byte(newNFC)) == nil {
			return ErrPasswordReused
		}
	}

	// Hash new password.
	newHash, err := bcrypt.GenerateFromPassword([]byte(newNFC), s.cfg.Security.BcryptCost)
	if err != nil {
		return fmt.Errorf("auth: hash new password: %w", err)
	}

	// Save old hash to history.
	if err := s.pwdHistoryRepo.Add(ctx, userID, user.PasswordHash); err != nil {
		return fmt.Errorf("auth: add to password history: %w", err)
	}

	// Update password.
	if err := s.userRepo.UpdatePassword(ctx, userID, string(newHash)); err != nil {
		return fmt.Errorf("auth: update password: %w", err)
	}

	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType: domain.EventAuthPasswordChanged,
		UserID:    &userID,
		ActorID:   &userID,
		Success:   true,
		IPAddress: req.IP,
		UserAgent: req.UserAgent,
	})
	return nil
}

// ValidatePasswordPolicy checks the new password against security policy.
func ValidatePasswordPolicy(password string) error {
	if utf8.RuneCountInString(password) < 10 {
		return fmt.Errorf("%w: password must be at least 10 characters", ErrPasswordPolicy)
	}

	var hasUpper, hasDigit, hasSymbol bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			hasSymbol = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("%w: password must contain at least one uppercase letter", ErrPasswordPolicy)
	}
	if !hasDigit {
		return fmt.Errorf("%w: password must contain at least one number", ErrPasswordPolicy)
	}
	if !hasSymbol {
		return fmt.Errorf("%w: password must contain at least one special character", ErrPasswordPolicy)
	}
	return nil
}

// HashPassword hashes a password with bcrypt at the configured cost.
// NFC normalization is applied before hashing.
func (s *AuthService) HashPassword(password string, skipPolicy bool) (string, error) {
	normalized := norm.NFC.String(password)
	if !skipPolicy {
		if err := ValidatePasswordPolicy(normalized); err != nil {
			return "", err
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(normalized), s.cfg.Security.BcryptCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(hash), nil
}

// AuthServiceI es una variante de AuthService que acepta interfaces en lugar de
// tipos de repositorio concretos. Es funcionalmente equivalente a AuthService y
// se usa en pruebas unitarias para permitir la inyección completa de mocks.
package service

import (
	"context"
	"fmt"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/unicode/norm"

	"github.com/enunezf/sentinel/internal/config"
	"github.com/enunezf/sentinel/internal/domain"
	redisrepo "github.com/enunezf/sentinel/internal/repository/redis"
	"github.com/enunezf/sentinel/internal/token"
)

// AuthServiceI encapsula la lógica de negocio de autenticación usando dependencias
// basadas en interfaces. Es el gemelo testeable de AuthService: ambos implementan
// exactamente la misma lógica; la única diferencia es que AuthServiceI recibe
// interfaces, lo que permite inyectar mocks en los tests sin necesidad de Docker.
type AuthServiceI struct {
	userRepo         UserRepositoryIface              // acceso a datos de usuarios
	appRepo          ApplicationRepositoryIface       // validación de X-App-Key
	refreshPGRepo    RefreshTokenPGRepositoryIface    // almacenamiento persistente de refresh tokens
	refreshRedisRepo RefreshTokenRedisRepositoryIface // caché de refresh tokens
	pwdHistoryRepo   PasswordHistoryRepositoryIface   // historial de contraseñas (últimas N)
	userRoleRepo     UserRoleRepositoryIface          // roles activos por usuario y aplicación
	tokenMgr         *token.Manager                  // generación y validación de JWT RS256
	auditSvc         AuditServiceIface                // registro asíncrono de eventos
	cfg              *config.Config                   // configuración de seguridad y JWT
}

// NewAuthServiceI crea un AuthServiceI con todas las dependencias basadas en interfaces.
// Todos los parámetros son obligatorios; un valor nil provocará un panic en tiempo de
// ejecución al intentar usarlos.
func NewAuthServiceI(
	userRepo UserRepositoryIface,
	appRepo ApplicationRepositoryIface,
	refreshPGRepo RefreshTokenPGRepositoryIface,
	refreshRedisRepo RefreshTokenRedisRepositoryIface,
	pwdHistoryRepo PasswordHistoryRepositoryIface,
	userRoleRepo UserRoleRepositoryIface,
	tokenMgr *token.Manager,
	auditSvc AuditServiceIface,
	cfg *config.Config,
) *AuthServiceI {
	return &AuthServiceI{
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

// Login autentica al usuario con su nombre de usuario y contraseña.
// El proceso sigue estos pasos:
//  1. Valida el X-App-Key (req.AppKey) para identificar la aplicación.
//  2. Valida que req.ClientType sea "web", "mobile" o "desktop".
//  3. Busca el usuario por username.
//  4. Verifica que la cuenta esté activa y no bloqueada.
//  5. Compara la contraseña (normalizada a NFC) con el hash bcrypt almacenado.
//  6. Si la contraseña falla, incrementa intentos y aplica lógica de lockout.
//  7. Genera un access token JWT RS256 y un refresh token UUID v4.
//  8. Almacena el refresh token (hash bcrypt en PG, metadatos en Redis).
//  9. Emite evento de auditoría (éxito o fallo).
//
// Parámetros:
//   - ctx: contexto de la solicitud HTTP.
//   - req: datos de entrada del login.
//
// Retorna LoginResponse con los tokens o uno de los errores de dominio:
// ErrApplicationNotFound, ErrInvalidClientType, ErrInvalidCredentials,
// ErrAccountInactive, ErrAccountLocked.
func (s *AuthServiceI) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	app, err := s.appRepo.FindBySecretKey(ctx, req.AppKey)
	if err != nil {
		return nil, fmt.Errorf("auth: find app: %w", err)
	}
	if app == nil || !app.IsActive {
		return nil, ErrApplicationNotFound
	}

	if !domain.IsValidClientType(req.ClientType) {
		return nil, ErrInvalidClientType
	}

	user, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("auth: find user: %w", err)
	}

	// Usuario no encontrado: registrar auditoría y devolver error genérico para evitar
	// enumeración de usuarios.
	if user == nil {
		appID := app.ID
		s.auditSvc.LogEvent(&domain.AuditLog{
			EventType:     domain.EventAuthLoginFailed,
			ApplicationID: &appID,
			IPAddress:     req.IP,
			UserAgent:     req.UserAgent,
			Success:       false,
			ErrorMessage:  "user not found",
		})
		return nil, ErrInvalidCredentials
	}

	if !user.IsActive {
		return nil, ErrAccountInactive
	}

	now := time.Now()
	if user.IsLocked(now) {
		return nil, ErrAccountLocked
	}

	// Normalizar la contraseña a Unicode NFC antes de comparar con el hash almacenado.
	// Esto garantiza que contraseñas con caracteres compuestos (tildes, etc.) sean
	// equivalentes independientemente de cómo las envía el cliente.
	normalizedPwd := norm.NFC.String(req.Password)
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(normalizedPwd)); err != nil {
		s.handleIFailedLogin(ctx, user, app.ID, req.IP, req.UserAgent)
		return nil, ErrInvalidCredentials
	}

	if err := s.userRepo.UpdateLastLogin(ctx, user.ID); err != nil {
		return nil, fmt.Errorf("auth: update last login: %w", err)
	}

	// El TTL del refresh token depende del tipo de cliente:
	// - web: 7 días (sesión de navegador, mayor riesgo de robo).
	// - mobile / desktop: 30 días (dispositivos de confianza).
	refreshTTL := s.cfg.JWT.RefreshTokenTTLWeb
	if req.ClientType == string(domain.ClientTypeMobile) || req.ClientType == string(domain.ClientTypeDesktop) {
		refreshTTL = s.cfg.JWT.RefreshTokenTTLMobile
	}

	roles, err := s.userRoleRepo.GetActiveRoleNamesForUserApp(ctx, user.ID, app.ID)
	if err != nil {
		return nil, fmt.Errorf("auth: get roles: %w", err)
	}

	accessToken, err := s.tokenMgr.GenerateAccessToken(user, app.Slug, roles, s.cfg.JWT.AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("auth: generate access token: %w", err)
	}

	// Generar un UUID v4 como refresh token raw; el valor raw se usa como clave en Redis
	// y se hashea con bcrypt para almacenarlo en PostgreSQL.
	rawRefreshToken := uuid.New().String()
	if err := s.istoreRefreshToken(ctx, user.ID, app.ID, rawRefreshToken, req.ClientType, req.IP, req.UserAgent, refreshTTL); err != nil {
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

// handleIFailedLogin gestiona un intento de login fallido (contraseña incorrecta).
// Incrementa el contador de intentos fallidos y aplica la política de bloqueo:
//   - Si se alcanza MaxFailedAttempts: se registra un lockout.
//   - Si es el tercer lockout en el mismo día: bloqueo permanente (locked_until = nil).
//   - Si es el primero o segundo lockout: bloqueo temporal con duración LockoutDuration.
//
// Siempre emite un evento de auditoría EventAuthLoginFailed. Si se produce un lockout,
// también emite EventAuthAccountLocked.
func (s *AuthServiceI) handleIFailedLogin(ctx context.Context, user *domain.User, appID uuid.UUID, ip, ua string) {
	user.FailedAttempts++
	var lockedUntil *time.Time
	lockoutCount := user.LockoutCount
	lockoutDate := user.LockoutDate
	now := time.Now()

	if user.FailedAttempts >= s.cfg.Security.MaxFailedAttempts {
		// Normalizar la fecha del lockout a medianoche UTC para contar lockouts por día.
		today := now.UTC().Truncate(24 * time.Hour)
		if lockoutDate == nil || !lockoutDate.UTC().Truncate(24*time.Hour).Equal(today) {
			// Primer lockout del día: reiniciar el contador diario.
			lockoutCount = 0
			lockoutDate = &today
		}
		lockoutCount++

		if lockoutCount >= 3 {
			// Tercer lockout en el mismo día: bloqueo permanente.
			// El campo locked_until = NULL con lockout_count >= 3 indica bloqueo permanente;
			// solo un administrador puede desbloquearlo manualmente.
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

// istoreRefreshToken hashea y persiste el refresh token en PostgreSQL y Redis.
// Proceso:
//  1. Genera el hash bcrypt del rawToken (costo configurado en cfg.Security.BcryptCost).
//  2. Crea el registro en PostgreSQL con el hash (el raw nunca se almacena en PG).
//  3. Guarda en Redis la clave "refresh:<rawToken>" con los metadatos y el hash,
//     para que una búsqueda posterior pueda encontrar el registro PG sin escanear
//     toda la tabla.
//
// Redis es no-fatal: si falla el Set, se ignora el error porque PostgreSQL es la
// fuente de verdad.
func (s *AuthServiceI) istoreRefreshToken(ctx context.Context, userID, appID uuid.UUID, rawToken, clientType, ip, ua string, ttl time.Duration) error {
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

	redisData := redisrepo.RefreshTokenData{
		UserID:     userID.String(),
		AppID:      appID.String(),
		ExpiresAt:  rt.ExpiresAt.Format(time.RFC3339),
		ClientType: clientType,
		UserAgent:  ua,
		IP:         ip,
		TokenHash:  hashStr, // almacenado en Redis para evitar el escaneo en PG
	}
	_ = s.refreshRedisRepo.Set(ctx, rawToken, redisData, ttl)
	return nil
}

// ifindRefreshToken localiza el registro de refresh token a partir del valor raw (UUID v4).
// Estrategia de búsqueda en dos pasos:
//  1. Consulta Redis por la clave "refresh:<rawToken>". Si existe y contiene el hash,
//     busca en PostgreSQL directamente por el hash (camino O(1)).
//  2. Si Redis falla o no tiene el hash, realiza un escaneo completo en PostgreSQL
//     comparando bcrypt (camino lento, O(n); aceptable para la escala actual).
func (s *AuthServiceI) ifindRefreshToken(ctx context.Context, rawToken string) (*domain.RefreshToken, error) {
	data, err := s.refreshRedisRepo.Get(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("auth: redis get refresh token: %w", err)
	}

	if data != nil && data.TokenHash != "" {
		// Camino rápido: Redis tiene el hash; buscar en PG por hash es O(1).
		rt, err := s.refreshPGRepo.FindByHash(ctx, data.TokenHash)
		if err != nil {
			return nil, fmt.Errorf("auth: pg find refresh by hash: %w", err)
		}
		return rt, nil
	}

	// Camino lento: Redis miss o hash ausente; PG hace la comparación bcrypt en todos los tokens.
	return s.refreshPGRepo.FindByRawToken(ctx, rawToken)
}

// Refresh valida un refresh token y emite un nuevo par de tokens (rotación de token).
// La rotación invalida el token anterior (revocación en PG y eliminación en Redis) y
// crea uno nuevo, manteniendo el mismo client_type y TTL de la sesión original.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - req: refresh token raw, app key, IP y User-Agent del cliente.
//
// Retorna RefreshResponse o uno de los errores: ErrApplicationNotFound, ErrTokenInvalid,
// ErrTokenRevoked, ErrTokenExpired, ErrAccountInactive, ErrAccountLocked.
func (s *AuthServiceI) Refresh(ctx context.Context, req RefreshRequest) (*RefreshResponse, error) {
	app, err := s.appRepo.FindBySecretKey(ctx, req.AppKey)
	if err != nil || app == nil || !app.IsActive {
		return nil, ErrApplicationNotFound
	}

	rtRecord, err := s.ifindRefreshToken(ctx, req.RefreshToken)
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

	// Verificar que el usuario siga activo y desbloqueado en el momento del refresh.
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

	// Revocar el token actual antes de emitir el nuevo (previene reutilización).
	if err := s.refreshPGRepo.Revoke(ctx, rtRecord.ID); err != nil {
		return nil, fmt.Errorf("auth: revoke refresh token: %w", err)
	}
	_ = s.refreshRedisRepo.Delete(ctx, req.RefreshToken)

	roles, err := s.userRoleRepo.GetActiveRoleNamesForUserApp(ctx, user.ID, rtRecord.AppID)
	if err != nil {
		return nil, fmt.Errorf("auth: get roles: %w", err)
	}

	accessToken, err := s.tokenMgr.GenerateAccessToken(user, app.Slug, roles, s.cfg.JWT.AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("auth: generate access token: %w", err)
	}

	rawRefreshToken := uuid.New().String()
	// Preservar el TTL según el client_type original de la sesión.
	ttl := s.cfg.JWT.RefreshTokenTTLWeb
	if rtRecord.DeviceInfo.ClientType == string(domain.ClientTypeMobile) ||
		rtRecord.DeviceInfo.ClientType == string(domain.ClientTypeDesktop) {
		ttl = s.cfg.JWT.RefreshTokenTTLMobile
	}

	if err := s.istoreRefreshToken(ctx, user.ID, rtRecord.AppID, rawRefreshToken, rtRecord.DeviceInfo.ClientType, req.IP, req.UserAgent, ttl); err != nil {
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

// Logout revoca todos los refresh tokens activos del usuario en la aplicación indicada.
// No invalida el access token (su vida corta es suficiente); el cliente debe descartarlo.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - claims: claims del access token del usuario autenticado.
//   - appKey: X-App-Key de la aplicación.
//   - ip, ua: dirección IP y User-Agent del cliente para la auditoría.
func (s *AuthServiceI) Logout(ctx context.Context, claims *domain.Claims, appKey, ip, ua string) error {
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

// ChangePassword permite al usuario autenticado cambiar su propia contraseña.
// Proceso:
//  1. Verifica la contraseña actual (normalizada a NFC) contra el hash almacenado.
//  2. Valida la nueva contraseña según la política (mínimo 10 caracteres, 1 mayúscula,
//     1 dígito, 1 símbolo).
//  3. Comprueba que la nueva contraseña no coincida con ninguna de las últimas N
//     contraseñas almacenadas en password_history.
//  4. Hashea la nueva contraseña con bcrypt (costo configurado).
//  5. Guarda el hash antiguo en password_history antes de actualizar.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - claims: claims del access token del usuario autenticado.
//   - req: contraseña actual, contraseña nueva, IP y User-Agent.
//
// Errores posibles: ErrInvalidCredentials, ErrPasswordPolicy, ErrPasswordReused.
func (s *AuthServiceI) ChangePassword(ctx context.Context, claims *domain.Claims, req ChangePasswordRequest) error {
	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return ErrInvalidCredentials
	}

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil || user == nil {
		return ErrInvalidCredentials
	}

	// Normalizar ambas contraseñas a NFC para consistencia con el hash almacenado.
	currentNFC := norm.NFC.String(req.CurrentPassword)
	newNFC := norm.NFC.String(req.NewPassword)

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentNFC)); err != nil {
		return ErrInvalidCredentials
	}

	if err := validatePasswordPolicyInternal(newNFC); err != nil {
		return err
	}

	// Recuperar los últimos N hashes del historial y comparar con la nueva contraseña.
	hashes, err := s.pwdHistoryRepo.GetLastN(ctx, userID, s.cfg.Security.PasswordHistory)
	if err != nil {
		return fmt.Errorf("auth: get password history: %w", err)
	}
	for _, h := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(h), []byte(newNFC)) == nil {
			return ErrPasswordReused
		}
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newNFC), s.cfg.Security.BcryptCost)
	if err != nil {
		return fmt.Errorf("auth: hash new password: %w", err)
	}

	// Guardar el hash actual en el historial antes de reemplazarlo.
	if err := s.pwdHistoryRepo.Add(ctx, userID, user.PasswordHash); err != nil {
		return fmt.Errorf("auth: add to password history: %w", err)
	}

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

// validatePasswordPolicyInternal es un alias privado de ValidatePasswordPolicy
// para que AuthServiceI pueda usarlo sin crear una dependencia circular en los tests.
// Aplica exactamente la misma política: mínimo 10 caracteres Unicode, al menos una
// letra mayúscula, un dígito y un carácter especial.
func validatePasswordPolicyInternal(password string) error {
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

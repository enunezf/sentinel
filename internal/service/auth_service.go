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

// Errores de dominio del servicio de autenticación. Estos valores se comparan con
// errors.Is en los handlers para determinar el código HTTP de respuesta adecuado.
var (
	// ErrInvalidCredentials se devuelve cuando el usuario no existe o la contraseña
	// no coincide. Se usa un error genérico para evitar enumeración de usuarios.
	ErrInvalidCredentials = errors.New("INVALID_CREDENTIALS")

	// ErrAccountInactive se devuelve cuando el usuario existe pero su campo is_active
	// es false. Un administrador debe reactivarlo explícitamente.
	ErrAccountInactive = errors.New("ACCOUNT_INACTIVE")

	// ErrAccountLocked se devuelve cuando el usuario está bloqueado temporalmente
	// (locked_until > NOW()) o permanentemente (locked_until = NULL con lockout_count >= 3).
	ErrAccountLocked = errors.New("ACCOUNT_LOCKED")

	// ErrTokenInvalid se devuelve cuando el refresh token no se encuentra en PG ni en Redis.
	ErrTokenInvalid = errors.New("TOKEN_INVALID")

	// ErrTokenExpired se devuelve cuando el refresh token existe pero su fecha de expiración
	// ya pasó. El cliente debe volver a hacer login.
	ErrTokenExpired = errors.New("TOKEN_EXPIRED")

	// ErrTokenRevoked se devuelve cuando el refresh token fue revocado explícitamente
	// (logout o rotación anterior). Indica posible reutilización del token.
	ErrTokenRevoked = errors.New("TOKEN_REVOKED")

	// ErrPasswordPolicy se devuelve cuando la nueva contraseña no cumple la política:
	// mínimo 10 caracteres, al menos una mayúscula, un dígito y un símbolo.
	ErrPasswordPolicy = errors.New("VALIDATION_ERROR")

	// ErrPasswordReused se devuelve cuando la nueva contraseña coincide con alguna
	// de las últimas N contraseñas almacenadas en password_history.
	ErrPasswordReused = errors.New("PASSWORD_REUSED")

	// ErrApplicationNotFound se devuelve cuando la secret_key enviada en X-App-Key
	// no corresponde a ninguna aplicación activa.
	ErrApplicationNotFound = errors.New("APPLICATION_NOT_FOUND")

	// ErrInvalidClientType se devuelve cuando el campo client_type no es uno de los
	// valores permitidos: "web", "mobile" o "desktop".
	ErrInvalidClientType = errors.New("INVALID_CLIENT_TYPE")
)

// AuthService implementa la lógica de negocio de autenticación usando repositorios
// concretos de PostgreSQL y Redis. Para pruebas unitarias se usa AuthServiceI, que
// acepta interfaces en lugar de tipos concretos.
type AuthService struct {
	userRepo         *postgres.UserRepository          // acceso a datos de usuarios
	appRepo          *postgres.ApplicationRepository   // validación de X-App-Key
	refreshPGRepo    *postgres.RefreshTokenRepository  // almacenamiento persistente de refresh tokens
	refreshRedisRepo *redisrepo.RefreshTokenRepository // caché de refresh tokens en Redis
	pwdHistoryRepo   *postgres.PasswordHistoryRepository // historial de contraseñas (últimas N)
	userRoleRepo     *postgres.UserRoleRepository      // roles activos por usuario y aplicación
	tokenMgr         *token.Manager                   // generación y validación de JWT RS256
	auditSvc         *AuditService                    // registro asíncrono de eventos de auditoría
	cfg              *config.Config                   // configuración de seguridad y JWT
}

// NewAuthService crea un AuthService con todos los repositorios y dependencias concretos.
// Se usa en producción; en tests se usa NewAuthServiceI.
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

// LoginRequest contiene los datos de entrada para la operación de login.
type LoginRequest struct {
	Username   string // nombre de usuario (sin transformar)
	Password   string // contraseña en texto plano (se normaliza a NFC internamente)
	ClientType string // tipo de cliente: "web" | "mobile" | "desktop"
	AppKey     string // valor del header X-App-Key para identificar la aplicación
	IP         string // dirección IP del cliente (para auditoría y bloqueo)
	UserAgent  string // cabecera User-Agent del cliente (para auditoría)
}

// LoginResponse contiene los tokens emitidos tras un login exitoso.
type LoginResponse struct {
	AccessToken  string       // JWT RS256 de corta duración (TTL según configuración)
	RefreshToken string       // UUID v4 raw del refresh token (no almacenar en localStorage en web)
	TokenType    string       // siempre "Bearer"
	ExpiresIn    int          // duración del access token en segundos
	User         *domain.User // datos del usuario autenticado
}

// Login autentica al usuario con su nombre de usuario y contraseña.
// El proceso sigue estos pasos:
//  1. Valida el X-App-Key (req.AppKey) para identificar la aplicación.
//  2. Valida que req.ClientType sea "web", "mobile" o "desktop".
//  3. Busca el usuario por username.
//  4. Verifica que la cuenta esté activa y no bloqueada.
//  5. Compara la contraseña (normalizada a NFC) con el hash bcrypt almacenado.
//  6. Si falla, incrementa intentos y aplica lógica de lockout.
//  7. Genera un access token JWT RS256 y un refresh token UUID v4.
//  8. Almacena el refresh token (hash bcrypt en PG, metadatos en Redis).
//  9. Emite evento de auditoría.
//
// Retorna LoginResponse o uno de: ErrApplicationNotFound, ErrInvalidClientType,
// ErrInvalidCredentials, ErrAccountInactive, ErrAccountLocked.
func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	// Paso 1: Validar X-App-Key.
	app, err := s.appRepo.FindBySecretKey(ctx, req.AppKey)
	if err != nil {
		return nil, fmt.Errorf("auth: find app: %w", err)
	}
	if app == nil || !app.IsActive {
		return nil, ErrApplicationNotFound
	}

	// Paso 1b: Validar client_type.
	if !domain.IsValidClientType(req.ClientType) {
		return nil, ErrInvalidClientType
	}

	// Paso 2: Buscar usuario por username.
	user, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("auth: find user: %w", err)
	}

	// Paso 3: Usuario no encontrado -> error genérico (evita enumeración de usuarios).
	if user == nil {
		appID := app.ID
		s.logAuditFailed(domain.EventAuthLoginFailed, nil, &appID, req.IP, req.UserAgent, "user not found")
		return nil, ErrInvalidCredentials
	}

	// Paso 4: Cuenta inactiva.
	if !user.IsActive {
		return nil, ErrAccountInactive
	}

	// Paso 5: Cuenta bloqueada.
	now := time.Now()
	if user.IsLocked(now) {
		return nil, ErrAccountLocked
	}

	// Paso 6: Comparar contraseña normalizada a NFC con el hash bcrypt almacenado.
	// La normalización NFC asegura que caracteres compuestos (ej. "é" como U+00E9 vs
	// U+0065+U+0301) sean tratados de forma equivalente.
	normalizedPwd := norm.NFC.String(req.Password)
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(normalizedPwd)); err != nil {
		// Paso 7: Contraseña incorrecta -> gestionar lockout.
		s.handleFailedLogin(ctx, user, app.ID, req.IP, req.UserAgent)
		return nil, ErrInvalidCredentials
	}

	// Paso 8: Login exitoso — actualizar last_login_at.
	if err := s.userRepo.UpdateLastLogin(ctx, user.ID); err != nil {
		return nil, fmt.Errorf("auth: update last login: %w", err)
	}

	// Determinar TTL del refresh token según el tipo de cliente:
	// web = 7 días, mobile/desktop = 30 días.
	refreshTTL := s.cfg.JWT.RefreshTokenTTLWeb
	if req.ClientType == string(domain.ClientTypeMobile) || req.ClientType == string(domain.ClientTypeDesktop) {
		refreshTTL = s.cfg.JWT.RefreshTokenTTLMobile
	}

	// Obtener los roles activos del usuario en esta aplicación para incluirlos en el JWT.
	roles, err := s.userRoleRepo.GetActiveRoleNamesForUserApp(ctx, user.ID, app.ID)
	if err != nil {
		return nil, fmt.Errorf("auth: get roles: %w", err)
	}

	// Generar el access token JWT RS256.
	accessToken, err := s.tokenMgr.GenerateAccessToken(user, app.Slug, roles, s.cfg.JWT.AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("auth: generate access token: %w", err)
	}

	// Generar el refresh token: UUID v4 raw como valor que se entrega al cliente.
	rawRefreshToken := uuid.New().String()

	// Almacenar el refresh token: hash bcrypt en PG, metadatos en Redis.
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

// handleFailedLogin incrementa el contador de intentos fallidos y aplica la política
// de lockout según la especificación:
//   - Al alcanzar MaxFailedAttempts: se registra un lockout para el día actual.
//   - Primer y segundo lockout del día: bloqueo temporal con duración LockoutDuration.
//   - Tercer lockout en el mismo día: bloqueo permanente (locked_until = NULL).
//
// Siempre emite EventAuthLoginFailed. Si hay lockout, también emite EventAuthAccountLocked.
// Los errores de base de datos se ignoran silenciosamente para no bloquear la respuesta.
func (s *AuthService) handleFailedLogin(ctx context.Context, user *domain.User, appID uuid.UUID, ip, ua string) {
	user.FailedAttempts++
	var lockedUntil *time.Time
	lockoutCount := user.LockoutCount
	lockoutDate := user.LockoutDate
	now := time.Now()

	if user.FailedAttempts >= s.cfg.Security.MaxFailedAttempts {
		// Truncar a medianoche UTC para comparar lockouts por día calendario.
		today := now.UTC().Truncate(24 * time.Hour)
		if lockoutDate == nil || !lockoutDate.UTC().Truncate(24*time.Hour).Equal(today) {
			// Nuevo día: reiniciar contador de lockouts diarios.
			lockoutCount = 0
			lockoutDate = &today
		}
		lockoutCount++

		if lockoutCount >= 3 {
			// Bloqueo permanente: locked_until = NULL con lockout_count >= 3.
			// Requiere intervención de un administrador para desbloquear.
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

// logAuditFailed es un helper para registrar eventos de auditoría de tipo fallo
// con un mensaje de error, sin bloquear el flujo principal.
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

// storeRefreshToken hashea y persiste el refresh token en PostgreSQL y Redis.
//
// Decisión de diseño: el UUID v4 raw se usa como clave en Redis
// ("refresh:<rawToken>"). El hash bcrypt se almacena en PostgreSQL como
// token_hash (UNIQUE). En la búsqueda, primero se consulta Redis con el token raw
// para obtener el hash, luego se busca en PG por hash. Esto cumple con la especificación
// (PG almacena hash bcrypt) y permite búsqueda O(1) en Redis sin necesitar un hash
// determinístico del token.
//
// Redis es no-fatal: si falla el Set, el error se ignora porque PostgreSQL es la
// fuente de verdad.
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

	// Clave Redis: el token raw (UUID v4). El valor incluye el hash bcrypt para
	// que la búsqueda posterior no requiera escanear toda la tabla de PG.
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
		// No fatal: PostgreSQL es fuente de verdad.
		_ = err
	}
	return nil
}

// findRefreshTokenByRaw localiza el registro de refresh token a partir del valor raw.
// Estrategia de dos pasos:
//  1. Consulta Redis por la clave "refresh:<rawToken>". Si existe y tiene hash,
//     busca en PG por hash (O(1)).
//  2. Cache miss: búsqueda lenta en PG con comparación bcrypt (O(n)).
//     Aceptable para ~2000 usuarios en v1; en producción se debería agregar
//     una columna de lookup SHA-256.
func (s *AuthService) findRefreshTokenByRaw(ctx context.Context, rawToken string) (*domain.RefreshToken, error) {
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

	// Camino lento: Redis miss; PG hace la comparación bcrypt en todos los tokens activos.
	return s.refreshPGRepo.FindByRawToken(ctx, rawToken)
}

// RefreshRequest contiene los datos de entrada para la rotación de tokens.
type RefreshRequest struct {
	RefreshToken string // UUID v4 raw del refresh token actual
	AppKey       string // valor del header X-App-Key
	IP           string // IP del cliente para auditoría
	UserAgent    string // User-Agent del cliente para auditoría
}

// RefreshResponse contiene el nuevo par de tokens emitido tras una rotación exitosa.
type RefreshResponse struct {
	AccessToken  string // nuevo JWT RS256
	RefreshToken string // nuevo UUID v4 raw del refresh token
	TokenType    string // siempre "Bearer"
	ExpiresIn    int    // duración del nuevo access token en segundos
}

// Refresh valida un refresh token y emite un nuevo par de tokens (rotación de token).
// El token actual se revoca en PostgreSQL y se elimina de Redis antes de emitir el nuevo.
// El nuevo refresh token hereda el mismo client_type y TTL de la sesión original.
//
// Retorna RefreshResponse o uno de: ErrApplicationNotFound, ErrTokenInvalid,
// ErrTokenRevoked, ErrTokenExpired, ErrAccountInactive, ErrAccountLocked.
func (s *AuthService) Refresh(ctx context.Context, req RefreshRequest) (*RefreshResponse, error) {
	// Validar App Key.
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

	// Verificar que el usuario siga activo y no bloqueado al momento del refresh.
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

	// Revocar el token actual (PG y Redis) antes de emitir el nuevo.
	if err := s.refreshPGRepo.Revoke(ctx, rtRecord.ID); err != nil {
		return nil, fmt.Errorf("auth: revoke refresh token: %w", err)
	}
	_ = s.refreshRedisRepo.Delete(ctx, req.RefreshToken)

	// Obtener roles activos para el nuevo access token.
	roles, err := s.userRoleRepo.GetActiveRoleNamesForUserApp(ctx, user.ID, rtRecord.AppID)
	if err != nil {
		return nil, fmt.Errorf("auth: get roles: %w", err)
	}

	// Obtener el slug de la aplicación para incluirlo en el access token.
	appObj, err := s.appRepo.FindBySecretKey(ctx, req.AppKey)
	if err != nil || appObj == nil {
		return nil, ErrApplicationNotFound
	}

	// Generar nuevo access token.
	accessToken, err := s.tokenMgr.GenerateAccessToken(user, appObj.Slug, roles, s.cfg.JWT.AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("auth: generate access token: %w", err)
	}

	rawRefreshToken := uuid.New().String()
	// Preservar el TTL del client_type original para mantener la sesión coherente.
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

// Logout revoca todos los refresh tokens activos del usuario en la aplicación indicada.
// El access token no se invalida explícitamente; su TTL corto es la protección suficiente.
// El cliente debe descartar ambos tokens localmente.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - claims: claims del JWT del usuario autenticado.
//   - appKey: valor del header X-App-Key.
//   - ip, ua: dirección IP y User-Agent para auditoría.
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

// ChangePasswordRequest contiene los datos de entrada para el cambio de contraseña.
type ChangePasswordRequest struct {
	CurrentPassword string // contraseña actual en texto plano (se normaliza a NFC internamente)
	NewPassword     string // nueva contraseña en texto plano (se normaliza a NFC internamente)
	IP              string // IP del cliente para auditoría
	UserAgent       string // User-Agent del cliente para auditoría
}

// ChangePassword permite al usuario autenticado cambiar su propia contraseña.
// Proceso:
//  1. Verifica la contraseña actual contra el hash bcrypt almacenado.
//  2. Valida la nueva contraseña según la política de seguridad.
//  3. Verifica que la nueva contraseña no haya sido usada en las últimas N iteraciones.
//  4. Hashea la nueva contraseña con bcrypt (costo configurado en cfg.Security.BcryptCost).
//  5. Guarda el hash antiguo en password_history antes de reemplazarlo.
//
// Errores posibles: ErrInvalidCredentials, ErrPasswordPolicy, ErrPasswordReused.
func (s *AuthService) ChangePassword(ctx context.Context, claims *domain.Claims, req ChangePasswordRequest) error {
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

	// Verificar contraseña actual.
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentNFC)); err != nil {
		return ErrInvalidCredentials
	}

	// Validar política de seguridad de la nueva contraseña.
	if err := ValidatePasswordPolicy(newNFC); err != nil {
		return err
	}

	// Verificar historial: la nueva contraseña no debe coincidir con las últimas N.
	hashes, err := s.pwdHistoryRepo.GetLastN(ctx, userID, s.cfg.Security.PasswordHistory)
	if err != nil {
		return fmt.Errorf("auth: get password history: %w", err)
	}
	for _, h := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(h), []byte(newNFC)) == nil {
			return ErrPasswordReused
		}
	}

	// Hashear la nueva contraseña con bcrypt al costo configurado.
	newHash, err := bcrypt.GenerateFromPassword([]byte(newNFC), s.cfg.Security.BcryptCost)
	if err != nil {
		return fmt.Errorf("auth: hash new password: %w", err)
	}

	// Guardar el hash actual en historial antes de reemplazarlo.
	if err := s.pwdHistoryRepo.Add(ctx, userID, user.PasswordHash); err != nil {
		return fmt.Errorf("auth: add to password history: %w", err)
	}

	// Persistir el nuevo hash en la base de datos.
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

// ValidatePasswordPolicy verifica que la contraseña cumpla la política de seguridad:
//   - Mínimo 10 caracteres Unicode (contados por runas, no bytes).
//   - Al menos una letra mayúscula.
//   - Al menos un dígito.
//   - Al menos un carácter que no sea letra ni dígito (símbolo o espacio).
//
// Retorna nil si la contraseña es válida, o un error wrapping ErrPasswordPolicy con
// el detalle del requisito incumplido.
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

// HashPassword hashea una contraseña con bcrypt al costo configurado.
// Aplica normalización NFC antes de hashear para garantizar que el mismo texto
// con diferente representación Unicode produzca siempre el mismo hash.
//
// Parámetros:
//   - password: contraseña en texto plano.
//   - skipPolicy: si es true, omite la validación de política (usado en bootstrap).
//
// Retorna el hash bcrypt como string o un error si el proceso falla.
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

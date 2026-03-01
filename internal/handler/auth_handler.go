// Package handler implementa los controladores HTTP de Sentinel usando Fiber v2.
// Cada handler recibe la request, valida los datos de entrada, delega la lógica
// al servicio correspondiente y responde con JSON. Los errores se normalizan
// siempre con respondError para mantener un formato uniforme en toda la API.
package handler

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/enunezf/sentinel/internal/middleware"
	"github.com/enunezf/sentinel/internal/service"
	"github.com/enunezf/sentinel/internal/token"
)

// AuthHandler agrupa los handlers de los endpoints de autenticación:
// login, refresh, logout y cambio de contraseña.
// Depende de AuthService para la lógica de negocio y de token.Manager
// para la generación del conjunto JWKS.
type AuthHandler struct {
	authSvc  *service.AuthService  // servicio que implementa la lógica de autenticación
	tokenMgr *token.Manager        // gestor de tokens JWT RS256 y JWKS
	logger   *slog.Logger          // logger estructurado con atributo component="auth"
}

// NewAuthHandler construye un AuthHandler inyectando sus dependencias.
// El logger recibido se enriquece con el atributo component="auth" para
// que todos los mensajes de este handler sean fácilmente identificables.
func NewAuthHandler(authSvc *service.AuthService, tokenMgr *token.Manager, log *slog.Logger) *AuthHandler {
	return &AuthHandler{
		authSvc:  authSvc,
		tokenMgr: tokenMgr,
		logger:   log.With("component", "auth"),
	}
}

// loginRequest es el cuerpo de la petición POST /auth/login.
// client_type determina el TTL del refresh token: 7 días para "web",
// 30 días para "mobile" y "desktop".
type loginRequest struct {
	Username   string `json:"username"`    // nombre de usuario, máximo 100 caracteres
	Password   string `json:"password"`    // contraseña en texto plano (se compara contra el hash bcrypt)
	ClientType string `json:"client_type"` // tipo de cliente: "web", "mobile" o "desktop"
}

// Login maneja POST /auth/login.
//
// Valida que username (1-100 chars), password y client_type estén presentes
// en el body JSON. Extrae el X-App-Key del header para identificar la aplicación
// cliente y el IP real del cliente (preferiendo X-Forwarded-For).
//
// En caso de éxito responde 200 con access_token, refresh_token, token_type,
// expires_in y los datos básicos del usuario. Si el usuario tiene la bandera
// must_change_password=true el frontend debe mostrar el diálogo de cambio de
// contraseña antes de permitir el acceso normal.
//
// Códigos HTTP posibles:
//   - 200: autenticación exitosa
//   - 400: body inválido, campo faltante o client_type desconocido
//   - 401: credenciales incorrectas o aplicación no registrada
//   - 403: cuenta inactiva o bloqueada por intentos fallidos
//   - 500: error interno inesperado
//
// @Summary     Iniciar sesión
// @Description Autentica un usuario con credenciales y retorna tokens JWT de acceso y refresco.
// @Tags        Autenticación
// @Accept      json
// @Produce     json
// @Param       X-App-Key  header   string                        true  "Clave secreta de la aplicación"
// @Param       body       body     SwaggerLoginRequest           true  "Credenciales de acceso"
// @Success     200        {object} SwaggerLoginResponse          "Autenticación exitosa"
// @Failure     400        {object} SwaggerErrorResponse          "Datos inválidos"
// @Failure     401        {object} SwaggerErrorResponse          "Credenciales incorrectas o aplicación no encontrada"
// @Failure     403        {object} SwaggerErrorResponse          "Cuenta inactiva o bloqueada"
// @Security    AppKeyAuth
// @Router      /auth/login [post]
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}

	if req.Username == "" || len(req.Username) > 100 {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "username is required and must be 1-100 characters")
	}
	if req.Password == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "password is required")
	}
	if req.ClientType == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "client_type is required")
	}

	resp, err := h.authSvc.Login(c.Context(), service.LoginRequest{
		Username:   req.Username,
		Password:   req.Password,
		ClientType: req.ClientType,
		AppKey:     c.Get("X-App-Key"),
		IP:         getIP(c),
		UserAgent:  c.Get("User-Agent"),
	})
	if err != nil {
		return h.mapAuthError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"access_token":  resp.AccessToken,
		"refresh_token": resp.RefreshToken,
		"token_type":    resp.TokenType,
		"expires_in":    resp.ExpiresIn,
		"user": fiber.Map{
			"id":                   resp.User.ID,
			"username":             resp.User.Username,
			"email":                resp.User.Email,
			"must_change_password": resp.User.MustChangePwd,
		},
	})
}

// refreshRequest es el cuerpo de la petición POST /auth/refresh.
type refreshRequest struct {
	RefreshToken string `json:"refresh_token"` // UUID v4 raw entregado por el login o el último refresh
}

// Refresh maneja POST /auth/refresh.
//
// Rota el par de tokens: invalida el refresh token actual en Redis y PostgreSQL
// y genera un nuevo access_token + refresh_token con el mismo TTL del cliente
// original. El nuevo refresh_token reemplaza completamente al anterior.
//
// El X-App-Key debe corresponder a la misma aplicación con la que se realizó
// el login que originó el refresh token.
//
// Códigos HTTP posibles:
//   - 200: tokens renovados exitosamente
//   - 400: body inválido o refresh_token vacío
//   - 401: token inválido, expirado o ya revocado; aplicación no registrada
//   - 500: error interno inesperado
//
// Refresh handles POST /auth/refresh.
//
// @Summary     Renovar tokens
// @Description Renueva el token de acceso usando un token de refresco válido.
// @Tags        Autenticación
// @Accept      json
// @Produce     json
// @Param       X-App-Key  header   string                  true  "Clave secreta de la aplicación"
// @Param       body       body     SwaggerRefreshRequest   true  "Token de refresco"
// @Success     200        {object} SwaggerTokenResponse    "Tokens renovados exitosamente"
// @Failure     400        {object} SwaggerErrorResponse    "Datos inválidos"
// @Failure     401        {object} SwaggerErrorResponse    "Token inválido, expirado o revocado"
// @Security    AppKeyAuth
// @Router      /auth/refresh [post]
func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	var req refreshRequest
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if req.RefreshToken == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "refresh_token is required")
	}

	resp, err := h.authSvc.Refresh(c.Context(), service.RefreshRequest{
		RefreshToken: req.RefreshToken,
		AppKey:       c.Get("X-App-Key"),
		IP:           getIP(c),
		UserAgent:    c.Get("User-Agent"),
	})
	if err != nil {
		return h.mapAuthError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"access_token":  resp.AccessToken,
		"refresh_token": resp.RefreshToken,
		"token_type":    resp.TokenType,
		"expires_in":    resp.ExpiresIn,
	})
}

// Logout maneja POST /auth/logout.
//
// Requiere JWT válido en el header Authorization (Bearer). Lee los claims
// del token desde c.Locals (inyectados por el middleware JWTAuth) y llama
// a AuthService.Logout para revocar el refresh token activo del usuario
// en Redis y en PostgreSQL.
//
// No requiere el refresh_token en el body: la revocación se hace en base
// al sub (user_id) del JWT y la aplicación identificada por X-App-Key.
//
// Códigos HTTP posibles:
//   - 204: sesión cerrada, sin cuerpo en la respuesta
//   - 401: JWT ausente, inválido o expirado
//   - 500: error interno inesperado
//
// Logout handles POST /auth/logout.
//
// @Summary     Cerrar sesión
// @Description Invalida el token de refresco del usuario autenticado.
// @Tags        Autenticación
// @Produce     json
// @Param       X-App-Key      header   string  true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true  "Token JWT. Formato: Bearer {token}"
// @Success     204            "Sesión cerrada exitosamente"
// @Failure     401            {object} SwaggerErrorResponse "No autenticado"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /auth/logout [post]
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "missing authentication")
	}

	if err := h.authSvc.Logout(c.Context(), claims, c.Get("X-App-Key"), getIP(c), c.Get("User-Agent")); err != nil {
		return h.mapAuthError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// changePasswordRequest es el cuerpo de la petición POST /auth/change-password.
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"` // contraseña actual para verificar identidad
	NewPassword     string `json:"new_password"`     // nueva contraseña que debe cumplir la política de seguridad
}

// ChangePassword maneja POST /auth/change-password.
//
// Requiere JWT válido. Verifica que current_password coincida con el hash
// almacenado antes de aceptar la nueva contraseña. La nueva contraseña debe
// cumplir la política de seguridad del sistema y no puede haber sido usada
// en las últimas 5 contraseñas (ver tabla password_history).
//
// La contraseña se normaliza a NFC con golang.org/x/text/unicode/norm
// antes del hashing bcrypt (costo 12) para garantizar consistencia con
// caracteres Unicode.
//
// Códigos HTTP posibles:
//   - 204: contraseña cambiada, sin cuerpo en la respuesta
//   - 400: body inválido, política de contraseña no cumplida o reutilización
//   - 401: JWT ausente, inválido o expirado
//   - 500: error interno inesperado
//
// ChangePassword handles POST /auth/change-password.
//
// @Summary     Cambiar contraseña
// @Description Cambia la contraseña del usuario autenticado. Requiere la contraseña actual.
// @Tags        Autenticación
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                        true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                        true  "Token JWT. Formato: Bearer {token}"
// @Param       body           body     SwaggerChangePasswordRequest  true  "Contraseñas actual y nueva"
// @Success     204            "Contraseña cambiada exitosamente"
// @Failure     400            {object} SwaggerErrorResponse          "Datos inválidos o política de contraseña"
// @Failure     401            {object} SwaggerErrorResponse          "No autenticado"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /auth/change-password [post]
func (h *AuthHandler) ChangePassword(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "missing authentication")
	}

	var req changePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if req.CurrentPassword == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "current_password is required")
	}
	if req.NewPassword == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "new_password is required")
	}

	if err := h.authSvc.ChangePassword(c.Context(), claims, service.ChangePasswordRequest{
		CurrentPassword: req.CurrentPassword,
		NewPassword:     req.NewPassword,
		IP:              getIP(c),
		UserAgent:       c.Get("User-Agent"),
	}); err != nil {
		return h.mapAuthError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// JWKS maneja GET /.well-known/jwks.json.
//
// Endpoint público (sin autenticación ni X-App-Key) que expone la clave pública
// RSA de Sentinel en formato JWKS (RFC 7517). Los backends consumidores utilizan
// esta clave para verificar localmente los tokens JWT RS256 sin necesidad de
// contactar a Sentinel en cada request.
//
// El JSON de respuesta incluye el campo "kid" (key ID) que permite a los
// consumidores identificar la clave correcta cuando haya rotación de claves.
//
// Códigos HTTP posibles:
//   - 200: JWKS con la clave pública RSA activa
//
// JWKS handles GET /.well-known/jwks.json.
//
// @Summary     Claves públicas JWKS
// @Description Retorna las claves públicas RSA en formato JWKS para verificación de tokens JWT.
// @Tags        Sistema
// @Produce     json
// @Success     200 {object} SwaggerJWKSResponse "Conjunto de claves públicas RSA"
// @Router      /.well-known/jwks.json [get]
func (h *AuthHandler) JWKS(c *fiber.Ctx) error {
	jwks := h.tokenMgr.GenerateJWKS()
	return c.Status(fiber.StatusOK).JSON(jwks)
}

// getIP extrae la IP real del cliente priorizando el header X-Forwarded-For,
// que es el que los proxies y load balancers incluyen con la IP de origen.
// Si ese header no existe, retorna la IP de la conexión TCP directa (c.IP()).
// Esta función es compartida por todos los handlers del paquete.
func getIP(c *fiber.Ctx) string {
	if ip := c.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	return c.IP()
}

// respondError escribe una respuesta de error JSON con el formato estándar de la API:
//
//	{"error": {"code": "CODIGO", "message": "descripcion", "details": null}}
//
// Todos los handlers y middlewares del paquete usan esta función para garantizar
// una estructura de error uniforme que el frontend puede procesar con un solo
// interceptor de errores. El campo "details" queda reservado para validaciones
// futuras con lista de campos inválidos.
func respondError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(fiber.Map{
		"error": fiber.Map{
			"code":    code,
			"message": message,
			"details": nil,
		},
	})
}

// mapAuthError traduce los errores del AuthService a respuestas HTTP con el
// código de estado y código de error apropiado. Centraliza el mapeo para que
// los handlers no contengan lógica de traducción de errores.
//
// Mapeo de errores:
//   - ErrApplicationNotFound  -> 401 APPLICATION_NOT_FOUND  (X-App-Key inválida)
//   - ErrInvalidClientType    -> 400 INVALID_CLIENT_TYPE    (valor fuera del enum web/mobile/desktop)
//   - ErrInvalidCredentials   -> 401 INVALID_CREDENTIALS    (usuario o contraseña incorrectos)
//   - ErrAccountInactive      -> 403 ACCOUNT_INACTIVE       (cuenta desactivada por un admin)
//   - ErrAccountLocked        -> 403 ACCOUNT_LOCKED         (bloqueo por intentos fallidos)
//   - ErrTokenInvalid         -> 401 TOKEN_INVALID          (token malformado o firma incorrecta)
//   - ErrTokenExpired         -> 401 TOKEN_EXPIRED          (token caducado)
//   - ErrTokenRevoked         -> 401 TOKEN_REVOKED          (token revocado manualmente o por logout)
//   - ErrPasswordReused       -> 400 PASSWORD_REUSED        (contraseña ya usada recientemente)
//   - ErrPasswordPolicy (wrap)-> 400 VALIDATION_ERROR       (política de contraseña no cumplida)
//   - cualquier otro error    -> 500 INTERNAL_ERROR         (error inesperado, se loguea como ERROR)
func (h *AuthHandler) mapAuthError(c *fiber.Ctx, err error) error {
	requestID, _ := c.Locals(middleware.LocalRequestID).(string)

	switch err {
	case service.ErrApplicationNotFound:
		// La clave X-App-Key no corresponde a ninguna aplicación registrada o activa.
		h.logger.Debug("auth error: application not found",
			"request_id", requestID,
			"ip", getIP(c),
		)
		return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", err.Error())
	case service.ErrInvalidClientType:
		// El campo client_type del body tiene un valor desconocido.
		h.logger.Debug("auth error: invalid client_type",
			"request_id", requestID,
		)
		return respondError(c, fiber.StatusBadRequest, "INVALID_CLIENT_TYPE", "client_type must be web, mobile, or desktop")
	case service.ErrInvalidCredentials:
		// Usuario no existe o la contraseña no coincide con el hash almacenado.
		// Se responde 401 genérico para no revelar qué dato es incorrecto.
		h.logger.Debug("auth error: invalid credentials",
			"request_id", requestID,
			"ip", getIP(c),
		)
		return respondError(c, fiber.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid username or password")
	case service.ErrAccountInactive:
		// El administrador desactivó la cuenta (is_active=false).
		h.logger.Info("auth error: account inactive",
			"request_id", requestID,
			"ip", getIP(c),
		)
		return respondError(c, fiber.StatusForbidden, "ACCOUNT_INACTIVE", "account is inactive")
	case service.ErrAccountLocked:
		// La cuenta fue bloqueada por superar el umbral de intentos fallidos.
		// El desbloqueo requiere acción de un administrador en POST /admin/users/:id/unlock.
		h.logger.Info("auth error: account locked",
			"request_id", requestID,
			"ip", getIP(c),
		)
		return respondError(c, fiber.StatusForbidden, "ACCOUNT_LOCKED", "account is locked")
	case service.ErrTokenInvalid:
		// El refresh token es malformado, tiene firma inválida o no existe en Redis.
		h.logger.Debug("auth error: token invalid",
			"request_id", requestID,
			"ip", getIP(c),
		)
		return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "invalid token")
	case service.ErrTokenExpired:
		// El refresh token superó su TTL configurado según el client_type.
		h.logger.Debug("auth error: token expired",
			"request_id", requestID,
			"ip", getIP(c),
		)
		return respondError(c, fiber.StatusUnauthorized, "TOKEN_EXPIRED", "token has expired")
	case service.ErrTokenRevoked:
		// El refresh token fue revocado explícitamente (logout o rotación previa).
		h.logger.Debug("auth error: token revoked",
			"request_id", requestID,
			"ip", getIP(c),
		)
		return respondError(c, fiber.StatusUnauthorized, "TOKEN_REVOKED", "token has been revoked")
	case service.ErrPasswordReused:
		// La nueva contraseña coincide con alguna de las últimas 5 usadas.
		h.logger.Debug("auth error: password reused",
			"request_id", requestID,
		)
		return respondError(c, fiber.StatusBadRequest, "PASSWORD_REUSED", "password was recently used")
	default:
		if isPasswordPolicyError(err) {
			// La nueva contraseña no cumple las reglas de complejidad (longitud,
			// mayúsculas, dígitos, caracteres especiales).
			h.logger.Debug("auth error: password policy violation",
				"request_id", requestID,
			)
			return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		}
		// Error no esperado: se loguea con nivel ERROR para alertar al equipo de operaciones.
		h.logger.Error("auth error: internal server error",
			"error", err,
			"request_id", requestID,
		)
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

// isPasswordPolicyError recorre la cadena de wrapping del error para determinar
// si en algún nivel se encuentra service.ErrPasswordPolicy. Esto permite que
// los servicios envuelvan el error con contexto adicional (ej. fmt.Errorf("...: %w", ErrPasswordPolicy))
// sin perder la capacidad de identificarlo en el handler.
func isPasswordPolicyError(err error) bool {
	if err == nil {
		return false
	}
	// Recorre la cadena de Unwrap hasta encontrar ErrPasswordPolicy o llegar al nil final.
	unwrapped := err
	for unwrapped != nil {
		if unwrapped == service.ErrPasswordPolicy {
			return true
		}
		unwrapped = unwrapErr(unwrapped)
	}
	return false
}

// unwrapErr extrae el error envuelto usando la interfaz Unwrap() estándar de Go.
// Retorna nil si el error no implementa Unwrap (es decir, es un error hoja).
func unwrapErr(err error) error {
	type unwrapper interface {
		Unwrap() error
	}
	if u, ok := err.(unwrapper); ok {
		return u.Unwrap()
	}
	return nil
}

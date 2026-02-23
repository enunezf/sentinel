package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/enunezf/sentinel/internal/middleware"
	"github.com/enunezf/sentinel/internal/service"
	"github.com/enunezf/sentinel/internal/token"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	authSvc  *service.AuthService
	tokenMgr *token.Manager
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authSvc *service.AuthService, tokenMgr *token.Manager) *AuthHandler {
	return &AuthHandler{authSvc: authSvc, tokenMgr: tokenMgr}
}

// loginRequest is the POST /auth/login request body.
type loginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	ClientType string `json:"client_type"`
}

// Login handles POST /auth/login.
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
		return mapAuthError(c, err)
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

// refreshRequest is the POST /auth/refresh request body.
type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

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
		return mapAuthError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"access_token":  resp.AccessToken,
		"refresh_token": resp.RefreshToken,
		"token_type":    resp.TokenType,
		"expires_in":    resp.ExpiresIn,
	})
}

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
		return mapAuthError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// changePasswordRequest is the POST /auth/change-password request body.
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

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
		return mapAuthError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// JWKS handles GET /.well-known/jwks.json.
//
// @Summary     Claves públicas JWKS
// @Description Retorna las claves públicas RSA en formato JWKS para verificación de tokens JWT.
// @Tags        Sistema
// @Produce     json
// @Success     200 {object} map[string]interface{} "Conjunto de claves JWKS"
// @Router      /.well-known/jwks.json [get]
func (h *AuthHandler) JWKS(c *fiber.Ctx) error {
	jwks := h.tokenMgr.GenerateJWKS()
	return c.Status(fiber.StatusOK).JSON(jwks)
}

// getIP extracts the client IP, preferring X-Forwarded-For.
func getIP(c *fiber.Ctx) string {
	if ip := c.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	return c.IP()
}

// respondError writes a standard error JSON response.
func respondError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(fiber.Map{
		"error": fiber.Map{
			"code":    code,
			"message": message,
			"details": nil,
		},
	})
}

// mapAuthError maps service errors to HTTP responses.
func mapAuthError(c *fiber.Ctx, err error) error {
	switch err {
	case service.ErrApplicationNotFound:
		return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", err.Error())
	case service.ErrInvalidClientType:
		return respondError(c, fiber.StatusBadRequest, "INVALID_CLIENT_TYPE", "client_type must be web, mobile, or desktop")
	case service.ErrInvalidCredentials:
		return respondError(c, fiber.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid username or password")
	case service.ErrAccountInactive:
		return respondError(c, fiber.StatusForbidden, "ACCOUNT_INACTIVE", "account is inactive")
	case service.ErrAccountLocked:
		return respondError(c, fiber.StatusForbidden, "ACCOUNT_LOCKED", "account is locked")
	case service.ErrTokenInvalid:
		return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "invalid token")
	case service.ErrTokenExpired:
		return respondError(c, fiber.StatusUnauthorized, "TOKEN_EXPIRED", "token has expired")
	case service.ErrTokenRevoked:
		return respondError(c, fiber.StatusUnauthorized, "TOKEN_REVOKED", "token has been revoked")
	case service.ErrPasswordReused:
		return respondError(c, fiber.StatusBadRequest, "PASSWORD_REUSED", "password was recently used")
	default:
		if isPasswordPolicyError(err) {
			return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		}
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func isPasswordPolicyError(err error) bool {
	if err == nil {
		return false
	}
	// Check if the error wraps ErrPasswordPolicy.
	unwrapped := err
	for unwrapped != nil {
		if unwrapped == service.ErrPasswordPolicy {
			return true
		}
		unwrapped = unwrapErr(unwrapped)
	}
	return false
}

func unwrapErr(err error) error {
	type unwrapper interface {
		Unwrap() error
	}
	if u, ok := err.(unwrapper); ok {
		return u.Unwrap()
	}
	return nil
}

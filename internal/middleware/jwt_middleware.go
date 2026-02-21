package middleware

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/token"
)

// JWTAuth validates the Bearer token in the Authorization header using RS256.
func JWTAuth(tokenMgr *token.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "missing Authorization header")
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "invalid Authorization header format")
		}

		tokenStr := parts[1]
		claims, err := tokenMgr.ValidateToken(tokenStr)
		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				return respondError(c, fiber.StatusUnauthorized, "TOKEN_EXPIRED", "access token has expired")
			}
			return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "invalid access token")
		}

		c.Locals(LocalClaims, claims)
		c.Locals(LocalActorID, claims.Sub)
		return c.Next()
	}
}

// GetClaims extracts JWT claims from fiber locals.
func GetClaims(c *fiber.Ctx) *domain.Claims {
	if v := c.Locals(LocalClaims); v != nil {
		if claims, ok := v.(*domain.Claims); ok {
			return claims
		}
	}
	return nil
}

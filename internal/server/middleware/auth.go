package middleware

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/Mark-0731/SwiftMail/internal/auth"
	"github.com/Mark-0731/SwiftMail/pkg/response"
)

// JWTAuth middleware validates JWT tokens from the Authorization header.
func JWTAuth(jwtManager *auth.JWTManager, logger zerolog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if header == "" {
			return response.Unauthorized(c, "Missing authorization header")
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return response.Unauthorized(c, "Invalid authorization format")
		}

		claims, err := jwtManager.ValidateAccessToken(parts[1])
		if err != nil {
			return response.Unauthorized(c, "Invalid or expired token")
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("user_email", claims.Email)
		c.Locals("user_role", claims.Role)

		return c.Next()
	}
}

// APIKeyAuth middleware validates API keys from the X-API-Key header.
func APIKeyAuth(apiKeyManager *auth.APIKeyManager, rdb *redis.Client, logger zerolog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		apiKey := c.Get("X-API-Key")
		if apiKey == "" {
			return response.Unauthorized(c, "Missing X-API-Key header")
		}

		keyHash := apiKeyManager.HashAPIKey(apiKey)

		// Check Redis cache first (hot path — zero DB hit)
		cached, err := apiKeyManager.GetCachedAPIKey(c.Context(), keyHash)
		if err == nil && cached != "" {
			var data auth.CachedAPIKeyData
			if err := json.Unmarshal([]byte(cached), &data); err == nil {
				if data.Status != "active" {
					return response.Forbidden(c, "Account is suspended")
				}
				c.Locals("user_id", data.UserID)
				c.Locals("user_role", data.Role)
				c.Locals("api_key_hash", keyHash)
				return c.Next()
			}
		}

		// Cache miss — this path should be rare
		return response.Unauthorized(c, "Invalid API key")
	}
}

// EitherAuth middleware accepts either JWT or API key authentication.
func EitherAuth(jwtManager *auth.JWTManager, apiKeyManager *auth.APIKeyManager, rdb *redis.Client, logger zerolog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Try JWT first
		if header := c.Get("Authorization"); header != "" {
			parts := strings.SplitN(header, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				claims, err := jwtManager.ValidateAccessToken(parts[1])
				if err == nil {
					c.Locals("user_id", claims.UserID)
					c.Locals("user_email", claims.Email)
					c.Locals("user_role", claims.Role)
					return c.Next()
				}
			}
		}

		// Try API key
		if apiKey := c.Get("X-API-Key"); apiKey != "" {
			keyHash := apiKeyManager.HashAPIKey(apiKey)
			cached, err := apiKeyManager.GetCachedAPIKey(c.Context(), keyHash)
			if err == nil && cached != "" {
				var data auth.CachedAPIKeyData
				if err := json.Unmarshal([]byte(cached), &data); err == nil {
					if data.Status != "active" {
						return response.Forbidden(c, "Account is suspended")
					}
					c.Locals("user_id", data.UserID)
					c.Locals("user_role", data.Role)
					c.Locals("api_key_hash", keyHash)
					return c.Next()
				}
			}
		}

		return response.Unauthorized(c, "Authentication required")
	}
}

// RequireRole middleware checks the user's role.
func RequireRole(roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("user_role").(string)
		for _, r := range roles {
			if role == r {
				return c.Next()
			}
		}
		return response.Forbidden(c, "Insufficient permissions")
	}
}

// GetUserID extracts the user ID from the Fiber context.
func GetUserID(c *fiber.Ctx) uuid.UUID {
	id, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return id
}

package middleware

import (
	"encoding/json"
	"strings"

	authdomain "github.com/Mark-0731/SwiftMail/internal/features/auth/domain"
	"github.com/Mark-0731/SwiftMail/pkg/response"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// JWTAuth middleware validates JWT tokens from the Authorization header.
func JWTAuth(jwtManager *authdomain.JWTManager, logger zerolog.Logger) fiber.Handler {
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
func APIKeyAuth(apiKeyManager *authdomain.APIKeyManager, rdb *redis.Client, logger zerolog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		apiKey := c.Get("X-API-Key")
		if apiKey == "" {
			return response.Unauthorized(c, "Missing X-API-Key header")
		}

		keyHash := apiKeyManager.HashAPIKey(apiKey)

		// Check Redis cache first (hot path — zero DB hit)
		cached, err := apiKeyManager.GetCachedAPIKey(c.Context(), keyHash)
		if err == nil && cached != "" {
			var data authdomain.CachedAPIKeyData
			if err := json.Unmarshal([]byte(cached), &data); err == nil {
				if data.Status != "active" {
					return response.Forbidden(c, "Account is suspended")
				}
				c.Locals("user_id", data.UserID)
				c.Locals("user_role", data.Role)
				c.Locals("api_key_hash", keyHash)
				c.Locals("api_key_permissions", data.Permissions)
				return c.Next()
			}
		}

		// Cache miss — load from database and cache it
		logger.Debug().Str("key_hash", keyHash).Msg("API key cache miss, loading from database")

		// Try to load from database and cache
		keyData, err := apiKeyManager.LoadAndCacheAPIKey(c.Context(), keyHash)
		if err != nil {
			logger.Warn().Err(err).Str("key_hash", keyHash).Msg("API key not found in database")
			return response.Unauthorized(c, "Invalid API key")
		}

		if keyData.Status != "active" {
			return response.Forbidden(c, "Account is suspended")
		}

		c.Locals("user_id", keyData.UserID)
		c.Locals("user_role", keyData.Role)
		c.Locals("api_key_hash", keyHash)
		c.Locals("api_key_permissions", keyData.Permissions)
		return c.Next()
	}
}

// EitherAuth middleware accepts either JWT or API key authentication.
func EitherAuth(jwtManager *authdomain.JWTManager, apiKeyManager *authdomain.APIKeyManager, rdb *redis.Client, logger zerolog.Logger) fiber.Handler {
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
				var data authdomain.CachedAPIKeyData
				if err := json.Unmarshal([]byte(cached), &data); err == nil {
					if data.Status != "active" {
						return response.Forbidden(c, "Account is suspended")
					}
					c.Locals("user_id", data.UserID)
					c.Locals("user_role", data.Role)
					c.Locals("api_key_hash", keyHash)
					c.Locals("api_key_permissions", data.Permissions)
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

// RequirePermission middleware checks if the API key has specific permissions.
func RequirePermission(permission string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Check if authenticated via API key
		_, ok := c.Locals("api_key_hash").(string)
		if !ok {
			// JWT auth doesn't have permission restrictions
			return c.Next()
		}

		// Get permissions from context (set by APIKeyAuth)
		permissions, ok := c.Locals("api_key_permissions").([]string)
		if !ok {
			// No permissions set, deny access
			return response.Forbidden(c, "API key does not have required permissions")
		}

		// Check if permission exists
		for _, p := range permissions {
			if p == permission || p == "*" {
				return c.Next()
			}
		}

		return response.Forbidden(c, "API key missing permission: "+permission)
	}
}

// RequireAnyPermission middleware checks if the API key has any of the specified permissions.
func RequireAnyPermission(permissions ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Check if authenticated via API key
		_, ok := c.Locals("api_key_hash").(string)
		if !ok {
			// JWT auth doesn't have permission restrictions
			return c.Next()
		}

		// Get permissions from context
		apiKeyPerms, ok := c.Locals("api_key_permissions").([]string)
		if !ok {
			return response.Forbidden(c, "API key does not have required permissions")
		}

		// Check if any permission matches
		for _, required := range permissions {
			for _, p := range apiKeyPerms {
				if p == required || p == "*" {
					return c.Next()
				}
			}
		}

		return response.Forbidden(c, "API key missing required permissions")
	}
}

package auth

import "github.com/gofiber/fiber/v2"

// RegisterPublicRoutes registers auth routes that don't require authentication.
func RegisterPublicRoutes(router fiber.Router, handler *Handler) {
	a := router.Group("/auth")
	a.Post("/register", handler.Signup)
	a.Post("/login", handler.Login)
	a.Post("/refresh", handler.RefreshToken)
}

// RegisterProtectedRoutes registers auth routes that require authentication.
func RegisterProtectedRoutes(router fiber.Router, handler *Handler) {
	a := router.Group("/auth")
	a.Get("/me", handler.GetProfile)
	a.Post("/totp/setup", handler.SetupTOTP)
	a.Post("/totp/verify", handler.VerifyTOTP)

	k := router.Group("/api-keys")
	k.Post("/", handler.CreateAPIKey)
	k.Get("/", handler.ListAPIKeys)
	k.Delete("/:id", handler.DeleteAPIKey)
}

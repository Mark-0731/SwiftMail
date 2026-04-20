package auth

import "github.com/gofiber/fiber/v2"

// RegisterPublicRoutes registers auth routes that don't require authentication.
func RegisterPublicRoutes(router fiber.Router, handler *Handler) {
	a := router.Group("/auth")

	// Authentication
	a.Post("/register", handler.Signup)
	a.Post("/login", handler.Login)
	a.Post("/refresh", handler.RefreshToken)

	// Password reset (public)
	a.Post("/password/reset-request", handler.RequestPasswordReset)
	a.Post("/password/reset", handler.ResetPassword)

	// Email verification (public)
	a.Post("/email/verify", handler.VerifyEmail)
}

// RegisterProtectedRoutes registers auth routes that require authentication.
func RegisterProtectedRoutes(router fiber.Router, handler *Handler) {
	a := router.Group("/auth")

	// Profile
	a.Get("/me", handler.GetProfile)

	// Password management
	a.Post("/password/change", handler.ChangePassword)

	// 2FA
	a.Post("/totp/setup", handler.SetupTOTP)
	a.Post("/totp/verify", handler.VerifyTOTP)

	// Email verification (resend)
	a.Post("/email/resend", handler.ResendVerificationEmail)

	// Session management
	a.Post("/sessions/revoke", handler.RevokeSession)
	a.Post("/sessions/revoke-all", handler.RevokeAllSessions)

	// API Keys
	k := router.Group("/api-keys")
	k.Post("/", handler.CreateAPIKey)
	k.Get("/", handler.ListAPIKeys)
	k.Delete("/:id", handler.DeleteAPIKey)
}

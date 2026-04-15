package smtprelay

import (
	"fmt"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/rs/zerolog"
	"github.com/swiftmail/swiftmail/internal/config"
)

// Server wraps the SMTP server with configuration.
type Server struct {
	smtp   *smtp.Server
	logger zerolog.Logger
}

// NewServer creates a new SMTP relay server.
func NewServer(cfg *config.Config, backend *Backend, logger zerolog.Logger) *Server {
	s := smtp.NewServer(backend)
	s.Addr = ":587" // Standard submission port
	s.Domain = "mail.swiftmail.local"
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 50 * 1024 * 1024 // 50MB
	s.MaxRecipients = 50
	s.AllowInsecureAuth = true // Allow PLAIN auth over non-TLS (for local dev)

	return &Server{
		smtp:   s,
		logger: logger,
	}
}

// ListenAndServe starts the SMTP server.
func (s *Server) ListenAndServe() error {
	s.logger.Info().
		Str("addr", s.smtp.Addr).
		Str("domain", s.smtp.Domain).
		Msg("SMTP relay server listening")

	fmt.Println("╔════════════════════════════════════════════════════════╗")
	fmt.Println("║       SwiftMail SMTP Relay Server Started             ║")
	fmt.Println("╠════════════════════════════════════════════════════════╣")
	fmt.Println("║  Port: 587 (Submission)                                ║")
	fmt.Println("║  Auth: API Key as username                             ║")
	fmt.Println("║  TLS:  Optional (STARTTLS supported)                   ║")
	fmt.Println("╚════════════════════════════════════════════════════════╝")

	return s.smtp.ListenAndServe()
}

// Close gracefully shuts down the server.
func (s *Server) Close() error {
	s.logger.Info().Msg("shutting down SMTP relay server...")
	return s.smtp.Close()
}

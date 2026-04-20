package smtprelay

import (
	"github.com/Mark-0731/SwiftMail/internal/features/auth/domain"
	"github.com/Mark-0731/SwiftMail/internal/features/auth/infrastructure"
	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// Backend implements SMTP server backend.
type Backend struct {
	db          *pgxpool.Pool
	rdb         *redis.Client
	asynqClient *asynq.Client
	authRepo    infrastructure.Repository
	apiKeyMgr   *domain.APIKeyManager
	logger      zerolog.Logger
}

// NewBackend creates a new SMTP backend.
func NewBackend(db *pgxpool.Pool, rdb *redis.Client, asynqClient *asynq.Client, authRepo infrastructure.Repository, apiKeyMgr *domain.APIKeyManager, logger zerolog.Logger) *Backend {
	return &Backend{
		db:          db,
		rdb:         rdb,
		asynqClient: asynqClient,
		authRepo:    authRepo,
		apiKeyMgr:   apiKeyMgr,
		logger:      logger,
	}
}

// NewSession creates a new SMTP session.
func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{
		backend: b,
		logger:  b.logger,
	}, nil
}

// Session represents an SMTP session.
type Session struct {
	backend       *Backend
	logger        zerolog.Logger
	userID        uuid.UUID
	from          string
	to            []string
	authenticated bool
}

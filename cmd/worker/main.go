package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/Mark-0731/SwiftMail/internal/config"
	emailrepo "github.com/Mark-0731/SwiftMail/internal/email/repository"
	"github.com/Mark-0731/SwiftMail/internal/events"
	"github.com/Mark-0731/SwiftMail/internal/provider"
	smtpengine "github.com/Mark-0731/SwiftMail/internal/smtp"
	"github.com/Mark-0731/SwiftMail/internal/worker"
	"github.com/Mark-0731/SwiftMail/pkg/logger"
	"github.com/Mark-0731/SwiftMail/pkg/metrics"
)

func main() {
	cfg := config.Load()
	log := logger.New(cfg.App.Env)
	log.Info().Msg("starting SwiftMail worker")

	ctx := context.Background()

	// Connect to PostgreSQL
	dbPool, err := pgxpool.New(ctx, cfg.Database.DSN())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to PostgreSQL")
	}
	defer dbPool.Close()
	log.Info().Msg("connected to PostgreSQL")

	// Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer rdb.Close()
	log.Info().Msg("connected to Redis")

	// Initialize metrics
	m := metrics.NewMetrics()

	// Initialize SMTP connection pool
	smtpPool, err := smtpengine.NewPool(&cfg.SMTP, log)
	if err != nil {
		log.Warn().Err(err).Msg("SMTP pool initialization had errors (Postfix may not be running)")
	}
	if smtpPool != nil {
		defer smtpPool.Close()
	}

	// Initialize SMTP components
	circuitBreaker := smtpengine.NewCircuitBreaker(rdb, log)
	mxResolver := smtpengine.NewMXResolver(rdb, log)
	smtpSender := smtpengine.NewSender(smtpPool, circuitBreaker, mxResolver, m, log)

	// Initialize email provider (SMTP as primary)
	smtpProvider := provider.NewSMTPProvider(smtpSender, log)

	// Optional: Add SendGrid as fallback for testing
	var emailProvider provider.Provider = smtpProvider
	if sendgridKey := os.Getenv("SENDGRID_API_KEY"); sendgridKey != "" {
		sendgridProvider := provider.NewSendGridProvider(sendgridKey, log)
		emailProvider = provider.NewSelector(smtpProvider, sendgridProvider, log)
		log.Info().Msg("SendGrid fallback enabled")
	} else {
		log.Info().Msg("using SMTP provider only")
	}

	// Initialize event bus
	eventBus := events.NewRedisBus(rdb, log)

	// Initialize repositories
	emailRepo := emailrepo.NewPostgresRepository(dbPool)

	// Initialize handlers
	sendHandler := worker.NewSendHandler(
		emailRepo,
		emailProvider,
		eventBus,
		m,
		cfg,
		log,
	)
	trackHandler := worker.NewTrackHandler(emailRepo, log)
	bounceHandler := worker.NewBounceHandler(emailRepo, log)

	// Create Asynq server
	srv := worker.NewServer(cfg, log)

	// Create mux
	mux := worker.NewMux(sendHandler, trackHandler, bounceHandler)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info().Int("concurrency", cfg.Worker.Concurrency).Msg("starting Asynq worker")
		if err := srv.Run(mux); err != nil {
			log.Fatal().Err(err).Msg("worker server error")
		}
	}()

	<-quit
	log.Info().Msg("shutting down worker...")
	srv.Shutdown()
	log.Info().Msg("worker stopped")
}

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/Mark-0731/SwiftMail/internal/config"
	"github.com/Mark-0731/SwiftMail/internal/features/auth/domain"
	"github.com/Mark-0731/SwiftMail/internal/features/auth/infrastructure"
	"github.com/Mark-0731/SwiftMail/internal/platform/smtprelay"
	"github.com/Mark-0731/SwiftMail/pkg/logger"
)

func main() {
	cfg := config.Load()
	log := logger.New(cfg.App.Env)
	log.Info().Msg("starting SwiftMail SMTP relay server")

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

	// Create Asynq client
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer asynqClient.Close()

	// Initialize auth components
	authRepo := infrastructure.NewPostgresRepository(dbPool)
	apiKeyMgr := domain.NewAPIKeyManager(rdb, authRepo)

	// Create SMTP backend
	backend := smtprelay.NewBackend(dbPool, rdb, asynqClient, authRepo, apiKeyMgr, log)

	// Create SMTP server
	server := smtprelay.NewServer(cfg, backend, log)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Fatal().Err(err).Msg("SMTP server error")
		}
	}()

	<-quit
	server.Close()
	log.Info().Msg("SMTP relay server stopped")
}

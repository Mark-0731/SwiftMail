package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"net/http"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/Mark-0731/SwiftMail/internal/config"
	warmup "github.com/Mark-0731/SwiftMail/internal/features/warmup/application"
	"github.com/Mark-0731/SwiftMail/internal/server"
	"github.com/Mark-0731/SwiftMail/pkg/database"
	"github.com/Mark-0731/SwiftMail/pkg/logger"
	"github.com/Mark-0731/SwiftMail/pkg/metrics"
)

func main() {
	// Load config
	cfg := config.Load()

	// Initialize logger
	log := logger.New(cfg.App.Env)
	log.Info().Str("env", cfg.App.Env).Msg("starting SwiftMail API server")

	ctx := context.Background()

	// Connect to PostgreSQL with Read Replica support
	dbPool, err := database.NewPool(ctx, cfg.Database.DSN(), cfg.Database.ReadReplicaDSN())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to PostgreSQL")
	}
	defer dbPool.Close()

	if err := dbPool.Ping(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to ping PostgreSQL")
	}
	log.Info().Msg("connected to PostgreSQL")
	if cfg.Database.ReadReplicaHost != "" {
		log.Info().Str("replica", cfg.Database.ReadReplicaHost).Msg("read replica enabled")
	}

	// Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal().Err(err).Msg("failed to connect to Redis")
	}
	log.Info().Msg("connected to Redis")

	// Connect to ClickHouse for analytics
	chConn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%s", cfg.ClickHouse.Host, cfg.ClickHouse.Port)},
		Auth: clickhouse.Auth{
			Database: cfg.ClickHouse.Database,
			Username: cfg.ClickHouse.User,
			Password: cfg.ClickHouse.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout: 5 * time.Second,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	})
	if err != nil {
		log.Warn().Err(err).Msg("failed to connect to ClickHouse (analytics will be disabled)")
	} else {
		if err := chConn.Ping(ctx); err != nil {
			log.Warn().Err(err).Msg("failed to ping ClickHouse")
		} else {
			log.Info().Msg("connected to ClickHouse")
		}
		defer chConn.Close()
	}

	// Start warmup scheduler (runs daily at midnight)
	warmupScheduler := warmup.NewScheduler(dbPool.GetPrimary(), log)
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		// Run immediately on startup
		if err := warmupScheduler.AdvanceWarmup(ctx); err != nil {
			log.Error().Err(err).Msg("warmup scheduler failed")
		}

		// Then run daily
		for range ticker.C {
			if err := warmupScheduler.AdvanceWarmup(context.Background()); err != nil {
				log.Error().Err(err).Msg("warmup scheduler failed")
			}
		}
	}()

	// Create Asynq client
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer asynqClient.Close()

	// Initialize Prometheus metrics
	m := metrics.NewMetrics()

	// Start Prometheus metrics server
	go func() {
		mux := http.NewServeMux()
		// Use custom registry to avoid metric duplication errors
		mux.Handle("/metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
		addr := fmt.Sprintf(":%s", cfg.Observability.PrometheusPort)
		log.Info().Str("addr", addr).Msg("starting Prometheus metrics server")
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Error().Err(err).Msg("metrics server error")
		}
	}()

	// Create Fiber server
	app := server.New(cfg, dbPool.GetPrimary(), rdb, asynqClient, m, log)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
		log.Info().Str("addr", addr).Msg("SwiftMail API server listening")
		if err := app.Listen(addr); err != nil {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	<-quit
	log.Info().Msg("shutting down gracefully...")
	app.Shutdown()
	log.Info().Msg("server stopped")
}

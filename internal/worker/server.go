package worker

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"github.com/Mark-0731/SwiftMail/internal/config"
)

// NewServer creates a new Asynq worker server with configured queues and concurrency.
func NewServer(cfg *config.Config, logger zerolog.Logger) *asynq.Server {
	asynqConfig := asynq.Config{
		Concurrency: cfg.Worker.Concurrency,
		Queues: map[string]int{
			"critical": cfg.Worker.QueueCritical,
			"high":     cfg.Worker.QueueHigh,
			"default":  cfg.Worker.QueueDefault,
			"low":      cfg.Worker.QueueLow,
		},
		// Retry configuration (Asynq handles retries)
		RetryDelayFunc: func(n int, err error, task *asynq.Task) time.Duration {
			// Exponential backoff: 1s, 2s, 4s, 8s, 16s
			return time.Duration(1<<uint(n)) * time.Second
		},
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			logger.Error().
				Err(err).
				Str("type", task.Type()).
				Str("task_id", task.ResultWriter().TaskID()).
				Msg("task processing failed")
		}),
		Logger: &asynqLogger{logger: logger},
	}

	// Apply strict priority if enabled
	if cfg.Worker.StrictPriority {
		asynqConfig.StrictPriority = true
	}

	// Apply shutdown timeout if configured
	if cfg.Worker.ShutdownTimeout > 0 {
		asynqConfig.ShutdownTimeout = cfg.Worker.ShutdownTimeout
	}

	return asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.Redis.Addr(),
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		},
		asynqConfig,
	)
}

// NewMux creates a new Asynq mux and registers all task handlers.
func NewMux(sendHandler *SendHandler, trackHandler *TrackHandler, bounceHandler *BounceHandler) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskEmailSend, sendHandler.ProcessTask)
	mux.HandleFunc(TaskTrackingEvent, trackHandler.ProcessTask)
	mux.HandleFunc(TaskBounceProcess, bounceHandler.ProcessTask)
	return mux
}

// asynqLogger adapts zerolog to asynq's logger interface.
type asynqLogger struct {
	logger zerolog.Logger
}

func (l *asynqLogger) Debug(args ...interface{}) {
	l.logger.Debug().Msgf("%v", args)
}

func (l *asynqLogger) Info(args ...interface{}) {
	l.logger.Info().Msgf("%v", args)
}

func (l *asynqLogger) Warn(args ...interface{}) {
	l.logger.Warn().Msgf("%v", args)
}

func (l *asynqLogger) Error(args ...interface{}) {
	l.logger.Error().Msgf("%v", args)
}

func (l *asynqLogger) Fatal(args ...interface{}) {
	l.logger.Fatal().Msgf("%v", args)
}

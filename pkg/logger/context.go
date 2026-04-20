package logger

import (
	"context"

	"github.com/rs/zerolog"
)

type contextKey string

const (
	loggerKey    contextKey = "logger"
	requestIDKey contextKey = "request_id"
)

// FromContext extracts the logger from context.
func FromContext(ctx context.Context) zerolog.Logger {
	if logger, ok := ctx.Value(loggerKey).(zerolog.Logger); ok {
		return logger
	}
	return zerolog.Nop()
}

// WithLogger adds a logger to the context.
func WithLogger(ctx context.Context, logger zerolog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// WithRequestID adds a request ID to the logger and context.
func WithRequestID(ctx context.Context, logger zerolog.Logger, requestID string) context.Context {
	loggerWithID := logger.With().Str("request_id", requestID).Logger()
	ctx = context.WithValue(ctx, loggerKey, loggerWithID)
	ctx = context.WithValue(ctx, requestIDKey, requestID)
	return ctx
}

// GetRequestID extracts the request ID from context.
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// WithFields adds additional fields to the logger in context.
func WithFields(ctx context.Context, fields map[string]interface{}) context.Context {
	logger := FromContext(ctx)
	logCtx := logger.With()

	for k, v := range fields {
		logCtx = logCtx.Interface(k, v)
	}

	return WithLogger(ctx, logCtx.Logger())
}

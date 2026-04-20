package errors

import (
	"github.com/Mark-0731/SwiftMail/pkg/response"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Must panics if err is not nil (use with defer recovery)
func Must(err error) {
	if err != nil {
		panic(err)
	}
}

// MustValue returns value or panics if err is not nil
func MustValue[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}

// LogError logs error and returns it
func LogError(logger zerolog.Logger, err error, msg string) error {
	if err != nil {
		logger.Error().Err(err).Msg(msg)
	}
	return err
}

// ParseUUID parses UUID from string and returns BadRequest on error
func ParseUUID(c *fiber.Ctx, id string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, response.BadRequest(c, "INVALID_ID", "Invalid ID format")
	}
	return parsed, nil
}

// HandleDBError returns appropriate HTTP error for database errors
func HandleDBError(c *fiber.Ctx, err error, msg string) error {
	if err != nil {
		return response.InternalError(c, msg)
	}
	return nil
}

// Chain executes functions in sequence, stops on first error
func Chain(fns ...func() error) error {
	for _, fn := range fns {
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}

// Try executes fn and returns error, useful for inline error handling
func Try(fn func() error) error {
	return fn()
}

// Ignore ignores the error (use sparingly)
func Ignore(_ error) {}

// OrPanic panics if err is not nil
func OrPanic(err error, msg string) {
	if err != nil {
		panic(msg + ": " + err.Error())
	}
}

// OrDefault returns defaultVal if err is not nil
func OrDefault[T any](val T, err error, defaultVal T) T {
	if err != nil {
		return defaultVal
	}
	return val
}

// WrapError wraps error with context message
func WrapError(err error, msg string) error {
	if err == nil {
		return nil
	}
	return &wrappedError{err: err, msg: msg}
}

type wrappedError struct {
	err error
	msg string
}

func (e *wrappedError) Error() string {
	return e.msg + ": " + e.err.Error()
}

func (e *wrappedError) Unwrap() error {
	return e.err
}

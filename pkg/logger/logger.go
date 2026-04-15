package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// New creates a new zerolog.Logger configured for the given environment.
func New(env string) zerolog.Logger {
	if env == "development" {
		return zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}).With().Timestamp().Caller().Logger()
	}

	return zerolog.New(os.Stdout).With().Timestamp().Logger()
}

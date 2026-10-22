// Package log sets up zerolog's global logger.
package log

import (
	"context"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Service embeds a zerolog logger, writing JSON on stdout.
type Service struct {
	zerolog.Logger
}

// Dial builds the logger at cfg's level, and sets zerolog's global logger to it.
func (s *Service) Dial(_ context.Context, cfg Config) error {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		return err
	}
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	s.Logger = zerolog.New(os.Stdout).Level(level).With().Timestamp().Logger()
	log.Logger = s.Logger

	return nil
}

// Close does nothing: stdout is not buffered.
func (s *Service) Close(context.Context) error {
	return nil
}

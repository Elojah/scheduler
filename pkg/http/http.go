// Package http serves HTTP, as a part a service dials and closes.
package http

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// readHeaderTimeout bounds how long a client takes to send a request's headers.
const readHeaderTimeout = 10 * time.Second

// Service serves Handler on Config.Address until Close. Set Handler before Dial.
type Service struct {
	Handler http.Handler

	srv *http.Server
}

// Dial listens on cfg.Address, so that an address in use fails here, and serves in the background.
func (s *Service) Dial(ctx context.Context, cfg Config) error {
	l, err := new(net.ListenConfig).Listen(ctx, "tcp", cfg.Address)
	if err != nil {
		return err
	}
	s.srv = &http.Server{Handler: s.Handler, ReadHeaderTimeout: readHeaderTimeout}
	go func() {
		if err := s.srv.Serve(l); !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("http stopped")
		}
	}()
	log.Info().Str("address", l.Addr().String()).Msg("http listening")

	return nil
}

// Close stops listening, and waits for the requests under way until ctx is done.
func (s *Service) Close(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}

	return s.srv.Shutdown(ctx)
}

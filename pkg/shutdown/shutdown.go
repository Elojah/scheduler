// Package shutdown closes a service's parts once it is signalled to stop.
package shutdown

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"
)

// Closer is a part of a service, closed when it stops.
type Closer interface {
	Close(ctx context.Context) error
}

// Closers are a service's parts, appended as they are dialed.
type Closers []Closer

// Close closes cs in reverse order, as defers run, so that a part closes before those it was dialed with.
func (cs Closers) Close(ctx context.Context) error {
	var err error
	for _, c := range slices.Backward(cs) {
		if c != nil {
			err = errors.Join(err, c.Close(ctx))
		}
	}

	return err
}

// WaitSignal blocks until SIGHUP, SIGINT or SIGTERM, then closes cs within to.
func (cs Closers) WaitSignal(to time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGHUP, os.Interrupt, syscall.SIGTERM)
	<-ctx.Done()
	stop()

	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()

	return cs.Close(ctx)
}

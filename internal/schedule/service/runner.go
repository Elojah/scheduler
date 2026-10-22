package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elojah/alarm"
	"github.com/elojah/alarm/holder"
	"github.com/elojah/alarm/middleware"
	"github.com/elojah/rrule"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/elojah/scheduler/internal/schedule"
)

// runner rings an alarm per enabled schedule, in a goroutine of its own.
type runner struct {
	actions schedule.Actions

	wg sync.WaitGroup
	// mu guards alarms, live and closed.
	mu sync.Mutex
	// alarms holds each schedule's alarm under way.
	alarms map[uuid.UUID]*ringing
	// live holds every alarm whose Ring has not returned, those replaced or stopped included, for shutdown to cancel.
	live map[*ringing]struct{}
	// closed is set by shutdown: no alarm starts after.
	closed bool
}

// ringing is a schedule's alarm.
type ringing struct {
	alarm *alarm.Alarm
	// cancel cancels Ring's ctx, which only shutdown does, once its own is done.
	cancel context.CancelFunc
	// tuning is what alarm.New took, which Reset cannot change.
	tuning tuning
	// schedule is what each time runs, which run swaps before a Reset.
	schedule atomic.Pointer[schedule.Schedule]
}

// tuning is what an alarm is built from: Options less Params.
type tuning struct {
	maxDelay, coalesce, spin, poll schedule.Duration
}

func newRunner(actions schedule.Actions) *runner {
	return &runner{actions: actions, alarms: map[uuid.UUID]*ringing{}, live: map[*ringing]struct{}{}}
}

// run rings s's alarm on set from now, or stops it if s is disabled. An alarm under way with the same tuning is Reset
// to set, and its action changed from the next time; otherwise it is closed and another one started. Ring's ctx is ctx
// without its cancellation, so that a request's ctx does not stop the alarm, which only stop and shutdown do.
func (r *runner) run(ctx context.Context, s schedule.Schedule, set rrule.Set) {
	if !s.Enabled {
		r.stop(s.ID)

		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}
	times := set.Between(time.Now(), time.Time{})
	t := tuningOf(s.Options)
	if old, ok := r.alarms[s.ID]; ok {
		if old.tuning == t {
			old.schedule.Store(&s)
			// False once the old rule ended: its Ring returns, so another one starts.
			if old.alarm.Reset(times) {
				return
			}
		}
		_ = old.alarm.Close()
	}

	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	rg := &ringing{alarm: alarm.New(r.options(s.ID, t)...), cancel: cancel, tuning: t}
	rg.schedule.Store(&s)
	r.alarms[s.ID] = rg
	r.live[rg] = struct{}{}
	r.wg.Go(func() {
		defer cancel()

		err := rg.alarm.Ring(ctx, times, func(ctx context.Context, at time.Time) {
			r.fire(ctx, rg.schedule.Load(), at)
		})
		log.Debug().Err(err).Stringer("id", s.ID).Msg("alarm stopped")

		r.mu.Lock()
		defer r.mu.Unlock()

		delete(r.live, rg)
		if r.alarms[s.ID] == rg {
			delete(r.alarms, s.ID)
		}
	})
}

// stop closes id's alarm, if any, without waiting for the action under way.
func (r *runner) stop(id uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if rg, ok := r.alarms[id]; ok {
		_ = rg.alarm.Close()
		delete(r.alarms, id)
	}
}

// shutdown closes every alarm and waits until their Rings return. Once ctx is done, it cancels their ctx, which
// actions get, and returns ctx.Err().
func (r *runner) shutdown(ctx context.Context) error {
	r.mu.Lock()
	r.closed = true
	for rg := range r.live {
		_ = rg.alarm.Close()
	}
	r.mu.Unlock()

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for rg := range r.live {
		rg.cancel()
	}

	return ctx.Err()
}

// fire runs s's action at at.
func (r *runner) fire(ctx context.Context, s *schedule.Schedule, at time.Time) {
	log.Debug().Stringer("id", s.ID).Str("action", s.Action).Time("at", at).Msg("fired")
	if err := r.actions[s.Action](ctx, *s, at); err != nil {
		log.Warn().Err(err).Stringer("id", s.ID).Str("action", s.Action).Time("at", at).Msg("action failed")
	}
}

// options returns the alarm options of t, leaving out zero ones, and recovering a panic in an action.
func (r *runner) options(id uuid.UUID, t tuning) []alarm.Option {
	dropped := func(_ context.Context, at time.Time, late time.Duration) {
		log.Debug().Stringer("id", id).Time("at", at).Dur("late", late).Msg("dropped")
	}
	middlewares := []alarm.Middleware{middleware.Recover(func(_ context.Context, at time.Time, v any) {
		log.Error().Stringer("id", id).Time("at", at).Interface("panic", v).Msg("action panicked")
	})}
	if t.maxDelay > 0 {
		middlewares = append(middlewares, middleware.MaxDelay(time.Duration(t.maxDelay), dropped))
	}

	options := []alarm.Option{alarm.WithMiddleware(middlewares...)}
	if t.coalesce > 0 {
		options = append(options, alarm.WithCoalesce(time.Duration(t.coalesce), dropped))
	}
	if t.spin > 0 {
		options = append(options, alarm.WithHolder(holder.NewSpin(time.Duration(t.spin))))
	}
	// WithPoll(0) would never poll.
	if t.poll > 0 {
		options = append(options, alarm.WithPoll(time.Duration(t.poll)))
	}

	return options
}

// tuningOf returns what o builds an alarm from.
func tuningOf(o schedule.Options) tuning {
	return tuning{maxDelay: o.MaxDelay, coalesce: o.Coalesce, spin: o.Spin, poll: o.Poll}
}

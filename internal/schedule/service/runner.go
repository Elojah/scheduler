package service

import (
	"bytes"
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

// runner rings an alarm per enabled schedule, in a goroutine of its own. Its methods run concurrently for different
// schedules, not for one, and shutdown alone.
type runner struct {
	actions schedule.Actions

	// alarms maps each enabled schedule's ID to its *ringing, whose Ring returned if its rule ended.
	alarms sync.Map
	// live maps each *ringing whose Ring has not returned, those replaced or stopped included, to the
	// context.CancelFunc of Ring's ctx, for shutdown to cancel.
	live sync.Map
	wg   sync.WaitGroup
	// closed is set by shutdown: no alarm starts after.
	closed atomic.Bool
}

// ringing is a schedule's alarm.
type ringing struct {
	alarm *alarm.Alarm
	// fn is what each time runs. The alarm holds it, so that update stores a new Action or Params into it, never
	// replacing it.
	fn *atomic.Pointer[func(ctx context.Context, at time.Time)]
	// schedule is the one last applied, for update to tell what changed. Its tuning is the one alarm.New took.
	schedule schedule.Schedule
}

// start rings s's alarm on set from now, closing the one before, if any. Ring's ctx is ctx without its cancellation,
// so that a request's ctx does not stop the alarm, which only stop and shutdown do.
func (r *runner) start(ctx context.Context, s schedule.Schedule, set rrule.Set) {
	if r.closed.Load() {
		return
	}

	times := set.Between(time.Now(), time.Time{})
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	rg := &ringing{alarm: alarm.New(r.options(s.ID, s.Options)...), fn: r.fire(s), schedule: s}
	old, _ := r.alarms.Swap(s.ID, rg)
	closeAlarm(old)
	r.live.Store(rg, cancel)
	r.wg.Go(func() {
		defer r.live.Delete(rg)
		defer cancel()

		err := rg.alarm.Ring(ctx, times, func(ctx context.Context, at time.Time) {
			(*rg.fn.Load())(ctx, at)
		})
		log.Debug().Err(err).Stringer("id", s.ID).Msg("alarm stopped")
	})
}

// update applies s to its alarm, doing only what changed: disabled, it stops the alarm; with none, or a new tuning,
// which Reset cannot change, it starts another. Else a new Action or Params swaps fn, and a new rule Resets the alarm.
func (r *runner) update(ctx context.Context, s schedule.Schedule, set rrule.Set) {
	if !s.Enabled {
		r.stop(s.ID)

		return
	}
	v, _ := r.alarms.Load(s.ID)
	rg, ok := v.(*ringing)
	if !ok || !rg.schedule.Options.SameTuning(s.Options) {
		r.start(ctx, s, set)

		return
	}

	old := rg.schedule
	rg.schedule = s
	if old.Action != s.Action || !bytes.Equal(old.Options.Params, s.Options.Params) {
		rg.fn.Store(r.fire(s).Load())
	}
	// False once the old rule ended: its Ring returned.
	if old.Rule != s.Rule && !rg.alarm.Reset(set.Between(time.Now(), time.Time{})) {
		r.start(ctx, s, set)
	}
}

// stop closes id's alarm, if any, without waiting for the action under way.
func (r *runner) stop(id uuid.UUID) {
	old, _ := r.alarms.LoadAndDelete(id)
	closeAlarm(old)
}

// shutdown closes every alarm, keeps others from starting, and waits until their Rings return. Once ctx is done, it
// cancels their ctx, which actions get, and returns ctx.Err().
func (r *runner) shutdown(ctx context.Context) error {
	r.closed.Store(true)
	r.alarms.Range(func(_, v any) bool {
		closeAlarm(v)

		return true
	})

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

	r.live.Range(func(_, v any) bool {
		if cancel, ok := v.(context.CancelFunc); ok {
			cancel()
		}

		return true
	})

	return ctx.Err()
}

// fire returns what each time of s runs: s's action, with s.
func (r *runner) fire(s schedule.Schedule) *atomic.Pointer[func(ctx context.Context, at time.Time)] {
	action := r.actions[s.Action]
	fn := func(ctx context.Context, at time.Time) {
		log.Debug().Stringer("id", s.ID).Str("action", s.Action).Time("at", at).Msg("fired")
		if err := action(ctx, s, at); err != nil {
			log.Warn().Err(err).Stringer("id", s.ID).Str("action", s.Action).Time("at", at).Msg("action failed")
		}
	}
	p := &atomic.Pointer[func(ctx context.Context, at time.Time)]{}
	p.Store(&fn)

	return p
}

// options returns the alarm options of o, leaving out zero ones, and recovering a panic in an action.
func (r *runner) options(id uuid.UUID, o schedule.Options) []alarm.Option {
	dropped := func(_ context.Context, at time.Time, late time.Duration) {
		log.Debug().Stringer("id", id).Time("at", at).Dur("late", late).Msg("dropped")
	}
	middlewares := []alarm.Middleware{middleware.Recover(func(_ context.Context, at time.Time, v any) {
		log.Error().Stringer("id", id).Time("at", at).Interface("panic", v).Msg("action panicked")
	})}
	if o.MaxDelay > 0 {
		middlewares = append(middlewares, middleware.MaxDelay(time.Duration(o.MaxDelay), dropped))
	}

	options := []alarm.Option{alarm.WithMiddleware(middlewares...)}
	if o.Coalesce > 0 {
		options = append(options, alarm.WithCoalesce(time.Duration(o.Coalesce), dropped))
	}
	if o.Spin > 0 {
		options = append(options, alarm.WithHolder(holder.NewSpin(time.Duration(o.Spin))))
	}
	// WithPoll(0) would never poll.
	if o.Poll > 0 {
		options = append(options, alarm.WithPoll(time.Duration(o.Poll)))
	}

	return options
}

// closeAlarm closes v's alarm, if v is a *ringing, without waiting for the action under way.
func closeAlarm(v any) {
	if rg, ok := v.(*ringing); ok {
		_ = rg.alarm.Close()
	}
}

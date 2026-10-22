// Package service runs an action at each time of a recurrence rule, for each schedule.Schedule a schedule.Store keeps.
//
//	svc := &service.S{Store: store, Actions: schedule.Blank()}
//	err := svc.Dial(ctx) // rings the enabled schedules stored
//	...
//	sc, err := svc.Create(ctx, schedule.Schedule{
//		Rule:    "DTSTART:20260101T090000Z\nRRULE:FREQ=DAILY;BYDAY=MO,TU,WE,TH,FR",
//		Action:  "noop",
//		Enabled: true,
//	})
//	...
//	err = svc.Close(ctx)
//
// Each enabled Schedule has an alarm (github.com/elojah/alarm) ringing its rule's times (github.com/elojah/rrule) from
// now, in a goroutine of its own, which runs the Action its key names. Update does only what changed: a new rule Resets
// the alarm, new Options but Params replace it, and a new Action or Params runs from the next time.
package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/elojah/scheduler/internal/schedule"
)

var _ schedule.Service = (*S)(nil)

// S keeps schedules in Store, and rings an alarm per enabled one, running one of Actions. Set Store and Actions, then
// Dial before any other call. Get and List are Store's.
//
// Its methods run concurrently, but changes of one schedule must not, else the store and the alarm could each keep a
// different one: the API serves one request at a time.
type S struct {
	schedule.Store

	Actions schedule.Actions

	runner *runner
}

// Dial rings the enabled schedules of the store, from now. It logs and skips a schedule that no longer runs, such as
// one whose action was removed.
func (s *S) Dial(ctx context.Context) error {
	s.runner = &runner{actions: s.Actions}
	schedules, err := s.List(ctx)
	if err != nil {
		return err
	}
	for _, sc := range schedules {
		if !sc.Enabled {
			continue
		}
		set, err := sc.Parse(s.Actions)
		if err != nil {
			log.Warn().Err(err).Stringer("id", sc.ID).Msg("schedule skipped")

			continue
		}
		s.runner.start(ctx, sc, set)
	}

	return nil
}

// Create stores sc under a new ID, and rings it if enabled. It returns sc with its ID, or an error of package schedule
// if sc cannot run.
func (s *S) Create(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return schedule.Schedule{}, err
	}
	sc.ID = id
	set, err := sc.Parse(s.Actions)
	if err != nil {
		return schedule.Schedule{}, err
	}

	if err := s.Store.Create(ctx, sc); err != nil {
		return schedule.Schedule{}, err
	}
	if sc.Enabled {
		s.runner.start(ctx, sc, set)
	}

	return sc, nil
}

// Update replaces the schedule of sc.ID with sc, and does only what changed to its alarm. A new rule Resets it from
// now, once the action under way returns. A new MaxDelay, Coalesce, Spin or Poll replaces it, and disabling stops it,
// the action under way finishing either way. A new Action or Params runs from the next time, which a passed time
// still is.
func (s *S) Update(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	set, err := sc.Parse(s.Actions)
	if err != nil {
		return schedule.Schedule{}, err
	}

	if err := s.Store.Update(ctx, sc); err != nil {
		return schedule.Schedule{}, err
	}
	s.runner.update(ctx, sc, set)

	return sc, nil
}

// Delete removes the schedule of id, and stops its alarm. The action under way finishes.
func (s *S) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.Store.Delete(ctx, id); err != nil {
		return err
	}
	s.runner.stop(id)

	return nil
}

// Close stops every alarm, and waits until the actions under way return. Once ctx is done, it cancels their ctx and
// returns ctx.Err(). Call it once no other call is under way: changes after it are stored, but ring no alarm.
func (s *S) Close(ctx context.Context) error {
	return s.runner.shutdown(ctx)
}

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
// now, in a goroutine of its own, which runs the Action its key names. Update Resets the alarm to the new rule, unless
// the alarm's Options changed, which replaces it.
package service

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/elojah/scheduler/internal/schedule"
)

var _ schedule.Service = (*S)(nil)

// S keeps schedules in Store, and rings an alarm per enabled one, running one of Actions. Set Store and Actions, then
// Dial before any other call. Get and List are Store's.
type S struct {
	schedule.Store

	Actions schedule.Actions

	runner *runner
	// mu serializes changes, so that the alarms run what the store holds.
	mu sync.Mutex
}

// Dial rings the enabled schedules of the store, from now. It logs and skips a schedule that no longer runs, such as
// one whose action was removed.
func (s *S) Dial(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.runner = newRunner(s.Actions)
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
		s.runner.run(ctx, sc, set)
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

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.Store.Create(ctx, sc); err != nil {
		return schedule.Schedule{}, err
	}
	s.runner.run(ctx, sc, set)

	return sc, nil
}

// Update replaces the schedule of sc.ID with sc. Its alarm is Reset to the new rule from now, unless MaxDelay,
// Coalesce, Spin or Poll changed, which replaces it, or sc is disabled, which stops it. The action under way finishes
// either way.
func (s *S) Update(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	set, err := sc.Parse(s.Actions)
	if err != nil {
		return schedule.Schedule{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.Store.Update(ctx, sc); err != nil {
		return schedule.Schedule{}, err
	}
	s.runner.run(ctx, sc, set)

	return sc, nil
}

// Delete removes the schedule of id, and stops its alarm. The action under way finishes.
func (s *S) Delete(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.Store.Delete(ctx, id); err != nil {
		return err
	}
	s.runner.stop(id)

	return nil
}

// Close stops every alarm, and waits until the actions under way return. Once ctx is done, it cancels their ctx and
// returns ctx.Err(). Changes after it are stored, but ring no alarm.
func (s *S) Close(ctx context.Context) error {
	return s.runner.shutdown(ctx)
}

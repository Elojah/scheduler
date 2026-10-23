package service_test

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/elojah/scheduler/internal/schedule"
	"github.com/elojah/scheduler/internal/schedule/service"
)

// epoch is where a bubble's clock starts.
var epoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

const (
	everySecond     = "DTSTART:20000101T000000Z\nRRULE:FREQ=SECONDLY"
	everyTwoSeconds = "DTSTART:20000101T000000Z\nRRULE:FREQ=SECONDLY;INTERVAL=2"
	twice           = "DTSTART:20000101T000000Z\nRRULE:FREQ=SECONDLY;COUNT=2"
	oddSeconds      = "DTSTART:20000101T000001Z\nRRULE:FREQ=SECONDLY;INTERVAL=2"
)

func TestMain(m *testing.M) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	os.Exit(m.Run())
}

// TestCreate: an enabled schedule runs its operation at each time of its rule, with its params, and a disabled one
// never.
func TestCreate(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc, fired := newService(t)
		a, err := svc.Create(t.Context(), schedule.Schedule{
			Rule: everySecond, Operation: "a", Enabled: true,
			Options: schedule.Options{Params: json.RawMessage(`{"relay":1}`)},
		})
		if err != nil {
			t.Fatal(err)
		}
		if a.ID == uuid.Nil {
			t.Error("Create assigned no ID")
		}
		if _, err := svc.Create(t.Context(), schedule.Schedule{Rule: everySecond, Operation: "b"}); err != nil {
			t.Fatal(err)
		}

		time.Sleep(2500 * time.Millisecond)
		fired.check(t, `a 0s {"relay":1}`, `a 1s {"relay":1}`, `a 2s {"relay":1}`)
		if got, err := svc.Get(t.Context(), a.ID); err != nil || got.Rule != everySecond {
			t.Errorf("Get = %+v, %v; want the schedule created", got, err)
		}
	})
}

// TestCreate_Invalid: a schedule that cannot run is refused with an error saying why, and not stored.
func TestCreate_Invalid(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		sc schedule.Schedule
		// want is a pointer to the error type Create returns.
		want any
	}{
		"rule without DTSTART": {schedule.Schedule{Rule: "FREQ=DAILY", Operation: "a"}, new(schedule.InvalidRRuleError)},
		"unknown operation":    {schedule.Schedule{Rule: everySecond, Operation: "z"}, new(schedule.UnknownOperationError)},
		"negative duration": {
			schedule.Schedule{Rule: everySecond, Operation: "a", Options: schedule.Options{MaxDelay: -1}},
			new(schedule.NegativeDurationError),
		},
		"params not JSON": {
			schedule.Schedule{Rule: everySecond, Operation: "a", Options: schedule.Options{Params: json.RawMessage("{")}},
			new(schedule.InvalidParamsError),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc, _ := newService(t)
			if _, err := svc.Create(t.Context(), tt.sc); !errors.As(err, tt.want) {
				t.Errorf("Create = %v, want a %T", err, tt.want)
			}
			if schedules, _ := svc.List(t.Context()); len(schedules) != 0 {
				t.Errorf("List = %+v, want none", schedules)
			}
		})
	}
}

// TestCreate_MaxDelay: a time passed by more than MaxDelay is dropped.
func TestCreate_MaxDelay(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc, fired := newService(t)
		// slow takes 1.5s: 1s and 3s are passed by 500ms once it returns.
		if _, err := svc.Create(t.Context(), schedule.Schedule{
			Rule: everySecond, Operation: "slow", Enabled: true,
			Options: schedule.Options{MaxDelay: schedule.Duration(100 * time.Millisecond)},
		}); err != nil {
			t.Fatal(err)
		}

		time.Sleep(4500 * time.Millisecond)
		fired.check(t, "slow 0s at 0s", "slow 2s at 2s", "slow 4s at 4s")
	})
}

// TestUpdate: a rule changes the times from now, an operation or params the next time run, options the alarm, and
// Enabled whether it rings. A rule that ended rings again.
func TestUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		created schedule.Schedule
		// updated is sent at 1.5s, or at 3.5s once a rule of twice ended.
		updated schedule.Schedule
		fired   []string
	}{{
		name:    "rule",
		created: schedule.Schedule{Rule: everySecond, Operation: "a", Enabled: true},
		updated: schedule.Schedule{Rule: everyTwoSeconds, Operation: "a", Enabled: true},
		fired:   []string{"a 0s", "a 1s", "a 2s", "a 4s"},
	}, {
		name:    "operation",
		created: schedule.Schedule{Rule: everySecond, Operation: "a", Enabled: true},
		updated: schedule.Schedule{
			Rule: everySecond, Operation: "b", Enabled: true,
			Options: schedule.Options{Params: json.RawMessage(`"on"`)},
		},
		fired: []string{"a 0s", "a 1s", `b 2s "on"`, `b 3s "on"`, `b 4s "on"`},
	}, {
		name:    "enable",
		created: schedule.Schedule{Rule: everySecond, Operation: "a"},
		updated: schedule.Schedule{Rule: everySecond, Operation: "a", Enabled: true},
		fired:   []string{"a 2s", "a 3s", "a 4s"},
	}, {
		name:    "disable",
		created: schedule.Schedule{Rule: everySecond, Operation: "a", Enabled: true},
		updated: schedule.Schedule{Rule: everySecond, Operation: "a"},
		fired:   []string{"a 0s", "a 1s"},
	}, {
		name:    "ended",
		created: schedule.Schedule{Rule: twice, Operation: "a", Enabled: true},
		updated: schedule.Schedule{Rule: everySecond, Operation: "a", Enabled: true},
		fired:   []string{"a 0s", "a 1s", "a 4s"},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				svc, fired := newService(t)
				sc, err := svc.Create(t.Context(), tt.created)
				if err != nil {
					t.Fatal(err)
				}

				wait := 1500 * time.Millisecond
				if tt.created.Rule == twice {
					wait = 3500 * time.Millisecond
				}
				time.Sleep(wait)
				tt.updated.ID = sc.ID
				if _, err := svc.Update(t.Context(), tt.updated); err != nil {
					t.Fatal(err)
				}

				time.Sleep(4500*time.Millisecond - wait)
				fired.check(t, tt.fired...)
				if got, _ := svc.Get(t.Context(), sc.ID); got.Rule != tt.updated.Rule || got.Enabled != tt.updated.Enabled {
					t.Errorf("Get = %+v, want %+v", got, tt.updated)
				}
			})
		})
	}
}

// TestUpdate_Reset: a new rule with the same options Resets the alarm: it reads the new times once the operation under
// way returns, rather than another alarm running them meanwhile, and the next time runs with the new params.
func TestUpdate_Reset(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc, fired := newService(t)
		sc, err := svc.Create(t.Context(), schedule.Schedule{Rule: everySecond, Operation: "slow", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(200 * time.Millisecond)
		sc.Rule, sc.Options.Params = oddSeconds, json.RawMessage(`"x"`)
		if _, err := svc.Update(t.Context(), sc); err != nil {
			t.Fatal(err)
		}

		time.Sleep(3300 * time.Millisecond)
		fired.check(t, "slow 0s at 0s", `slow 1s at 1.5s "x"`, `slow 3s at 3s "x"`)
	})
}

// TestUpdate_Params: new params alone leave the alarm's times as they were: one passed while the operation runs still
// runs, with them, where a Reset from now would drop it.
func TestUpdate_Params(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc, fired := newService(t)
		sc, err := svc.Create(t.Context(), schedule.Schedule{Rule: everySecond, Operation: "slow", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(1200 * time.Millisecond)
		sc.Options.Params = json.RawMessage(`"x"`)
		if _, err := svc.Update(t.Context(), sc); err != nil {
			t.Fatal(err)
		}

		time.Sleep(2100 * time.Millisecond)
		fired.check(t, "slow 0s at 0s", `slow 1s at 1.5s "x"`, `slow 2s at 3s "x"`)
	})
}

// TestUpdate_Options: options that build the alarm replace it, which a Reset could not do: compare with
// TestUpdate_Reset.
func TestUpdate_Options(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc, fired := newService(t)
		sc, err := svc.Create(t.Context(), schedule.Schedule{Rule: everySecond, Operation: "slow", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(200 * time.Millisecond)
		sc.Options.MaxDelay = schedule.Duration(100 * time.Millisecond)
		if _, err := svc.Update(t.Context(), sc); err != nil {
			t.Fatal(err)
		}

		// The new alarm runs 1s while the old one finishes 0s, then drops 2s, passed by 500ms at 2.5s.
		time.Sleep(3300 * time.Millisecond)
		fired.check(t, "slow 0s at 0s", "slow 1s at 1s", "slow 3s at 3s")
	})
}

// TestUpdate_Invalid: a refused update leaves the schedule, and its alarm, as they were.
func TestUpdate_Invalid(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc, fired := newService(t)
		sc, err := svc.Create(t.Context(), schedule.Schedule{Rule: everySecond, Operation: "a", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Update(t.Context(), schedule.Schedule{ID: sc.ID, Rule: "nope", Operation: "a"}); !errors.As(
			err, new(schedule.InvalidRRuleError)) {
			t.Errorf("Update = %v, want an InvalidRRuleError", err)
		}
		unknown := schedule.Schedule{ID: uuid.New(), Rule: everySecond, Operation: "a"}
		if _, err := svc.Update(t.Context(), unknown); !errors.Is(err, schedule.NotFoundError{ID: unknown.ID}) {
			t.Errorf("Update of an unknown ID = %v, want a NotFoundError", err)
		}

		time.Sleep(1500 * time.Millisecond)
		fired.check(t, "a 0s", "a 1s")
	})
}

// TestDelete: a deleted schedule stops ringing, and is no longer found.
func TestDelete(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc, fired := newService(t)
		sc, err := svc.Create(t.Context(), schedule.Schedule{Rule: everySecond, Operation: "a", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(1500 * time.Millisecond)
		if err := svc.Delete(t.Context(), sc.ID); err != nil {
			t.Fatal(err)
		}

		time.Sleep(3 * time.Second)
		fired.check(t, "a 0s", "a 1s")
		if _, err := svc.Get(t.Context(), sc.ID); !errors.Is(err, schedule.NotFoundError{ID: sc.ID}) {
			t.Errorf("Get = %v, want a NotFoundError", err)
		}
		if err := svc.Delete(t.Context(), sc.ID); !errors.Is(err, schedule.NotFoundError{ID: sc.ID}) {
			t.Errorf("Delete again = %v, want a NotFoundError", err)
		}
	})
}

// TestDial: the enabled schedules stored ring from now, and one that no longer runs is skipped.
func TestDial(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		store := newStore()
		for _, sc := range []schedule.Schedule{
			{Rule: everySecond, Operation: "a", Enabled: true},
			{Rule: everySecond, Operation: "b"},
			{Rule: everySecond, Operation: "removed", Enabled: true},
		} {
			sc.ID = uuid.Must(uuid.NewV7())
			if err := store.Create(t.Context(), sc); err != nil {
				t.Fatal(err)
			}
		}
		time.Sleep(1500 * time.Millisecond)

		_, fired := newServiceWith(t, store)

		time.Sleep(time.Second)
		fired.check(t, "a 2s")
	})
}

// TestParallel: changes of different schedules run concurrently, and leave no alarm once deleted.
func TestParallel(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc, fired := newService(t)
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				sc, err := svc.Create(t.Context(), schedule.Schedule{Rule: everySecond, Operation: "a", Enabled: true})
				if err != nil {
					t.Error(err)

					return
				}
				for _, change := range []func(){
					func() { sc.Rule = everyTwoSeconds },
					func() { sc.Operation = "b" },
					func() { sc.Options.MaxDelay = schedule.Duration(time.Second) },
					func() { sc.Enabled = false },
					func() { sc.Enabled = true },
				} {
					change()
					if _, err := svc.Update(t.Context(), sc); err != nil {
						t.Error(err)
					}
				}
				if err := svc.Delete(t.Context(), sc.ID); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		synctest.Wait()

		before := fired.count()
		time.Sleep(3 * time.Second)
		if after := fired.count(); after != before {
			t.Errorf("fired %d times after every schedule was deleted", after-before)
		}
	})
}

// TestParallel_Delete: updates of one schedule racing its Delete leave no alarm, whichever stored first.
func TestParallel_Delete(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc, fired := newService(t)
		sc, err := svc.Create(t.Context(), schedule.Schedule{Rule: everySecond, Operation: "a", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		for _, change := range []func(sc *schedule.Schedule){
			func(sc *schedule.Schedule) { sc.Rule = everyTwoSeconds },
			func(sc *schedule.Schedule) { sc.Operation = "b" },
			func(sc *schedule.Schedule) { sc.Options.MaxDelay = schedule.Duration(time.Second) },
			func(sc *schedule.Schedule) { sc.Enabled = false },
		} {
			wg.Go(func() {
				sc := sc
				change(&sc)
				if _, err := svc.Update(t.Context(), sc); err != nil && !errors.Is(err, schedule.NotFoundError{ID: sc.ID}) {
					t.Error(err)
				}
			})
		}
		wg.Go(func() {
			if err := svc.Delete(t.Context(), sc.ID); err != nil {
				t.Error(err)
			}
		})
		wg.Wait()
		synctest.Wait()

		before := fired.count()
		time.Sleep(3 * time.Second)
		if after := fired.count(); after != before {
			t.Errorf("fired %d times after the schedule was deleted", after-before)
		}
	})
}

// TestClose: Close waits for the operations under way, and cancels their ctx once its own is done.
func TestClose(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc, fired := newService(t)
		if _, err := svc.Create(t.Context(), schedule.Schedule{Rule: everySecond, Operation: "stuck", Enabled: true}); err != nil {
			t.Fatal(err)
		}

		time.Sleep(500 * time.Millisecond)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		if err := svc.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Close = %v, want context.DeadlineExceeded", err)
		}

		synctest.Wait()
		fired.check(t, "stuck 0s", "stuck 0s canceled at 1.5s")
	})
}

// TestClose_Idle: Close returns nil once the alarms stopped, and no schedule rings after.
func TestClose_Idle(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc, fired := newService(t)
		if _, err := svc.Create(t.Context(), schedule.Schedule{Rule: everySecond, Operation: "a", Enabled: true}); err != nil {
			t.Fatal(err)
		}

		time.Sleep(500 * time.Millisecond)
		if err := svc.Close(t.Context()); err != nil {
			t.Errorf("Close = %v, want nil", err)
		}
		if _, err := svc.Create(t.Context(), schedule.Schedule{Rule: everySecond, Operation: "b", Enabled: true}); err != nil {
			t.Fatal(err)
		}

		time.Sleep(2 * time.Second)
		fired.check(t, "a 0s")
	})
}

// newService returns a dialed S on a store of its own, closed at the end of the test, with these operations, logging to
// fired:
//   - a and b log their name, the time since epoch and their params, if any;
//   - slow logs as a, with when it runs, then sleeps 1.5s;
//   - stuck logs as a, then waits until ctx is done, and logs it with when.
func newService(t *testing.T) (*service.S, *log) {
	t.Helper()

	return newServiceWith(t, newStore())
}

func newServiceWith(t *testing.T, store schedule.Store) (*service.S, *log) {
	t.Helper()

	l := &log{}
	logged := func(name string) schedule.Operation {
		return func(_ context.Context, s schedule.Schedule, at time.Time) error {
			l.add(fmt.Sprintf("%s %v", name, at.Sub(epoch)), s.Options.Params)

			return nil
		}
	}
	operations := schedule.Operations{
		"a": logged("a"),
		"b": logged("b"),
		"slow": func(_ context.Context, s schedule.Schedule, at time.Time) error {
			l.add(fmt.Sprintf("slow %v at %v", at.Sub(epoch), time.Since(epoch)), s.Options.Params)
			time.Sleep(1500 * time.Millisecond)

			return nil
		},
		"stuck": func(ctx context.Context, s schedule.Schedule, at time.Time) error {
			_ = logged("stuck")(ctx, s, at)
			<-ctx.Done()
			l.addLine(fmt.Sprintf("stuck %v %v at %v", at.Sub(epoch), "canceled", time.Since(epoch)))

			return ctx.Err()
		},
	}
	svc := &service.S{Store: store, Operations: operations}
	if err := svc.Dial(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = svc.Close(context.Background())
	})

	return svc, l
}

// log keeps the lines the operations log.
type log struct {
	mu    sync.Mutex
	lines []string
}

// add logs line, then params if any.
func (l *log) add(line string, params json.RawMessage) {
	if len(params) > 0 {
		line += " " + string(params)
	}
	l.addLine(line)
}

func (l *log) addLine(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.lines = append(l.lines, line)
}

func (l *log) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return len(l.lines)
}

func (l *log) check(t *testing.T, want ...string) {
	t.Helper()

	l.mu.Lock()
	defer l.mu.Unlock()

	if !slices.Equal(l.lines, want) {
		t.Errorf("fired %q, want %q", l.lines, want)
	}
}

// store is a schedule.Store in memory.
type store struct {
	mu        sync.Mutex
	schedules map[uuid.UUID]schedule.Schedule
}

func newStore() *store {
	return &store{schedules: map[uuid.UUID]schedule.Schedule{}}
}

func (s *store) Create(_ context.Context, sc schedule.Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.schedules[sc.ID] = sc

	return nil
}

func (s *store) Get(_ context.Context, id uuid.UUID) (schedule.Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sc, ok := s.schedules[id]
	if !ok {
		return schedule.Schedule{}, schedule.NotFoundError{ID: id}
	}

	return sc, nil
}

func (s *store) List(context.Context) ([]schedule.Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.SortedFunc(maps.Values(s.schedules), func(a, b schedule.Schedule) int {
		return cmp.Compare(a.ID.String(), b.ID.String())
	}), nil
}

func (s *store) Update(_ context.Context, sc schedule.Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.schedules[sc.ID]; !ok {
		return schedule.NotFoundError{ID: sc.ID}
	}
	s.schedules[sc.ID] = sc

	return nil
}

func (s *store) Delete(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.schedules[id]; !ok {
		return schedule.NotFoundError{ID: id}
	}
	delete(s.schedules, id)

	return nil
}

package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/elojah/scheduler/internal/schedule"
	schedulesqlite "github.com/elojah/scheduler/internal/schedule/sqlite"
	"github.com/elojah/scheduler/pkg/sqlite"
)

// TestStore: a schedule reads back as written, options included, through Create, Update and a reopening.
func TestStore(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "schedule.db")
	s := open(t, path)
	sc := schedule.Schedule{
		ID:     uuid.Must(uuid.NewV7()),
		Rule:   "DTSTART:20260101T090000Z\nRRULE:FREQ=DAILY",
		Action: "noop",
		Options: schedule.Options{
			MaxDelay: schedule.Duration(time.Second),
			Coalesce: schedule.Duration(time.Minute),
			Spin:     schedule.Duration(time.Millisecond),
			Poll:     schedule.Duration(30 * time.Second),
			Params:   json.RawMessage(`{"relay":1}`),
		},
		Enabled: true,
	}
	if err := s.Create(t.Context(), sc); err != nil {
		t.Fatal(err)
	}
	get(t, s, sc)

	sc.Rule, sc.Action, sc.Options, sc.Enabled = "DTSTART:20260101T090000Z\nRRULE:FREQ=HOURLY", "other", schedule.Options{}, false
	if err := s.Update(t.Context(), sc); err != nil {
		t.Fatal(err)
	}
	get(t, s, sc)

	if err := s.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	get(t, open(t, path), sc)
}

// TestStore_List: List returns every schedule in the order of their UUIDv7, which is that of creation.
func TestStore_List(t *testing.T) {
	t.Parallel()

	s := open(t, filepath.Join(t.TempDir(), "schedule.db"))
	if schedules, err := s.List(t.Context()); err != nil || len(schedules) != 0 {
		t.Errorf("List = %+v, %v; want none", schedules, err)
	}

	want := make([]uuid.UUID, 3)
	for i := range want {
		want[i] = uuid.Must(uuid.NewV7())
	}
	// Inserted out of order.
	for _, i := range []int{2, 0, 1} {
		if err := s.Create(t.Context(), schedule.Schedule{ID: want[i], Rule: "r", Action: "a"}); err != nil {
			t.Fatal(err)
		}
	}

	schedules, err := s.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := make([]uuid.UUID, 0, len(schedules))
	for _, sc := range schedules {
		got = append(got, sc.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
}

// TestStore_NotFound: Get, Update and Delete return a NotFoundError for an ID the store does not hold, deleted or never
// created.
func TestStore_NotFound(t *testing.T) {
	t.Parallel()

	s := open(t, filepath.Join(t.TempDir(), "schedule.db"))
	sc := schedule.Schedule{ID: uuid.Must(uuid.NewV7()), Rule: "r", Action: "a"}
	if err := s.Create(t.Context(), sc); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(t.Context(), sc.ID); err != nil {
		t.Fatal(err)
	}

	for _, id := range []uuid.UUID{sc.ID, uuid.New()} {
		notFound := schedule.NotFoundError{ID: id}
		if _, err := s.Get(t.Context(), id); !errors.Is(err, notFound) {
			t.Errorf("Get = %v, want %v", err, notFound)
		}
		if err := s.Update(t.Context(), schedule.Schedule{ID: id}); !errors.Is(err, notFound) {
			t.Errorf("Update = %v, want %v", err, notFound)
		}
		if err := s.Delete(t.Context(), id); !errors.Is(err, notFound) {
			t.Errorf("Delete = %v, want %v", err, notFound)
		}
	}
}

// TestStore_Create: an ID created twice is refused.
func TestStore_Create(t *testing.T) {
	t.Parallel()

	s := open(t, filepath.Join(t.TempDir(), "schedule.db"))
	sc := schedule.Schedule{ID: uuid.Must(uuid.NewV7()), Rule: "r", Action: "a"}
	if err := s.Create(t.Context(), sc); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(t.Context(), sc); err == nil {
		t.Error("Create of an ID held = nil, want an error")
	}
}

// open returns a Store on the database at path, closed at the end of the test.
func open(t *testing.T, path string) *schedulesqlite.Store {
	t.Helper()

	db := sqlite.Service{}
	if err := db.Dial(t.Context(), sqlite.Config{Path: path}); err != nil {
		t.Fatal(err)
	}
	s := &schedulesqlite.Store{Service: db}
	t.Cleanup(func() {
		_ = s.Close(context.Background())
	})
	if err := s.Init(t.Context()); err != nil {
		t.Fatal(err)
	}

	return s
}

func get(t *testing.T, s *schedulesqlite.Store, want schedule.Schedule) {
	t.Helper()

	got, err := s.Get(t.Context(), want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Get = %+v, want %+v", got, want)
	}
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/elojah/scheduler/internal/schedule"
	"github.com/elojah/scheduler/internal/schedule/service"
	schedulesqlite "github.com/elojah/scheduler/internal/schedule/sqlite"
	"github.com/elojah/scheduler/pkg/sqlite"
)

const (
	// timeout bounds each wait on an operation.
	timeout = 5 * time.Second
	// every100ms starts at a whole second, so that it lines up with any clock.
	every100ms = "DTSTART:20260101T000000Z\nRRULE:FREQ=SECONDLY;X-FREQ=PT0.1S"
)

var errDown = errors.New("store down")

type reply struct {
	Schedule  *schedule.Schedule  `json:"schedule"`
	Schedules []schedule.Schedule `json:"schedules"`
	Error     *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestMain(m *testing.M) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	os.Exit(m.Run())
}

// TestServe: each op does what the Service does, and replies with the result.
func TestServe(t *testing.T) {
	t.Parallel()

	h, _ := newHandler(t)
	if r := call(t, h, "list", `{}`); r.Error != nil || r.Schedules == nil || len(r.Schedules) != 0 {
		t.Errorf("list = %+v, want no error and []", r)
	}

	r := call(t, h, "create", `{"rule":"`+jsonLine(every100ms)+`","operation":"noop","options":{"max_delay":"50ms"}}`)
	if r.Error != nil || r.Schedule == nil {
		t.Fatalf("create = %+v, want a schedule", r)
	}
	sc := *r.Schedule
	if sc.Rule != every100ms || sc.Operation != "noop" || sc.Options.MaxDelay != schedule.Duration(50*time.Millisecond) ||
		sc.Enabled {
		t.Errorf("create = %+v, want what was sent", sc)
	}

	if r := call(t, h, "get", fmt.Sprintf(`{"id":%q}`, sc.ID)); r.Error != nil || r.Schedule.ID != sc.ID {
		t.Errorf("get = %+v, want the schedule created", r)
	}
	r = call(t, h, "update", fmt.Sprintf(`{"id":%q,"rule":"%s","operation":"noop","enabled":true}`, sc.ID, jsonLine(every100ms)))
	if r.Error != nil || !r.Schedule.Enabled {
		t.Errorf("update = %+v, want the schedule enabled", r)
	}
	if r := call(t, h, "list", `{}`); r.Error != nil || len(r.Schedules) != 1 || !r.Schedules[0].Enabled {
		t.Errorf("list = %+v, want the schedule updated", r)
	}
	if r := call(t, h, "delete", fmt.Sprintf(`{"id":%q}`, sc.ID)); r.Error != nil || r.Schedule != nil {
		t.Errorf("delete = %+v, want {}", r)
	}
	if r := call(t, h, "get", fmt.Sprintf(`{"id":%q}`, sc.ID)); r.Error == nil || r.Error.Code != "not_found" {
		t.Errorf("get after delete = %+v, want not_found", r)
	}
}

// TestServe_Invalid: a request that cannot be served replies with a code saying why.
func TestServe_Invalid(t *testing.T) {
	t.Parallel()

	h, _ := newHandler(t)
	for _, tt := range []struct{ op, payload, code string }{
		{"create", `{`, "invalid"},
		{"create", `{"rule":"FREQ=DAILY","operation":"noop"}`, "invalid"},
		{"create", `{"rule":"` + jsonLine(every100ms) + `","operation":"unknown"}`, "invalid"},
		{"create", `{"rule":"` + jsonLine(every100ms) + `","operation":"noop","enable":true}`, "invalid"},
		{"create", `{"rule":"` + jsonLine(every100ms) + `","operation":"noop","options":{"poll":"-1s"}}`, "invalid"},
		{"create", `{"rule":"` + jsonLine(every100ms) + `","operation":"noop","options":{"max_delay":"soon"}}`, "invalid"},
		{"get", `{"id":"not a uuid"}`, "invalid"},
		{"get", `{"id":"0199f0a8-0000-7000-8000-000000000000"}`, "not_found"},
		{"update", `{"id":"0199f0a8-0000-7000-8000-000000000000","rule":"` + jsonLine(every100ms) + `","operation":"noop"}`,
			"not_found"},
		{"delete", `{"id":"0199f0a8-0000-7000-8000-000000000000"}`, "not_found"},
		{"rename", `{}`, "invalid"},
	} {
		if r := call(t, h, tt.op, tt.payload); r.Error == nil || r.Error.Code != tt.code {
			t.Errorf("%s %s = %+v, want %s", tt.op, tt.payload, r, tt.code)
		}
	}
}

// TestServe_Internal: an error of no known type replies internal, with its message.
func TestServe_Internal(t *testing.T) {
	t.Parallel()

	h := &handler{schedule: &schedule.ServiceMock{
		ListFunc: func(context.Context) ([]schedule.Schedule, error) {
			return nil, errDown
		},
	}}
	if r := call(t, h, "list", `{}`); r.Error == nil || r.Error.Code != "internal" || r.Error.Message != errDown.Error() {
		t.Errorf("list = %+v, want internal", r)
	}
}

// TestServe_Fires: a schedule created enabled runs its operation.
func TestServe_Fires(t *testing.T) {
	t.Parallel()

	h, fired := newHandler(t)
	if r := call(t, h, "create", `{"rule":"`+jsonLine(every100ms)+`","operation":"probe","enabled":true}`); r.Error != nil {
		t.Fatalf("create = %+v", r)
	}
	for range 3 {
		select {
		case <-fired:
		case <-time.After(timeout):
			t.Fatal("the operation never ran")
		}
	}
}

// newHandler returns a handler on a service.S on a database of its own, closed at the end of the test, with a probe
// operation sending on fired.
func newHandler(t *testing.T) (*handler, <-chan struct{}) {
	t.Helper()

	db := sqlite.Service{}
	if err := db.Dial(t.Context(), sqlite.Config{Path: filepath.Join(t.TempDir(), "scheduler.db")}); err != nil {
		t.Fatal(err)
	}
	store := &schedulesqlite.Store{Service: db}
	if err := store.Init(t.Context()); err != nil {
		t.Fatal(err)
	}

	fired := make(chan struct{}, 64)
	operations := schedule.Blank()
	operations["probe"] = func(context.Context, schedule.Schedule, time.Time) error {
		select {
		case fired <- struct{}{}:
		default:
		}

		return nil
	}
	svc := &service.S{Store: store, Operations: operations}
	if err := svc.Dial(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := svc.Close(ctx); err != nil {
			t.Errorf("Close = %v", err)
		}
		_ = db.Close(ctx)
	})

	return &handler{schedule: svc}, fired
}

// call serves payload on op and returns the reply.
func call(t *testing.T, h *handler, op, payload string) reply {
	t.Helper()

	var r reply
	if err := json.Unmarshal(h.serve(t.Context(), op, []byte(payload)), &r); err != nil {
		t.Fatalf("%s: %v", op, err)
	}

	return r
}

// jsonLine escapes the line feeds of a rule for a JSON string.
func jsonLine(rule string) string {
	return strings.ReplaceAll(rule, "\n", `\n`)
}

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/elojah/scheduler/internal/schedule"
	"github.com/elojah/scheduler/pkg/mqtt"
)

const (
	hourly   = "DTSTART:20260101T000000Z\nRRULE:FREQ=HOURLY"
	minutely = "DTSTART:20260101T090000Z\nRRULE:FREQ=MINUTELY;COUNT=60"
	daily    = "DTSTART:20260101T090000Z\nRRULE:FREQ=DAILY"
	// every100ms starts at a whole second, so that it lines up with any clock.
	every100ms = "DTSTART:20260101T000000Z\nRRULE:FREQ=SECONDLY;X-FREQ=PT0.1S"
	// day is the range of the occurrences tested.
	day = "from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z"
)

// fake is a requester replying with its func.
type fake func(op string, payload []byte) ([]byte, error)

func (f fake) Request(_ context.Context, op string, payload []byte) ([]byte, error) {
	return f(op, payload)
}

type reply struct {
	Occurrences []occurrence   `json:"occurrences"`
	Times       []time.Time    `json:"times"`
	Error       *responseError `json:"error"`
}

func TestMain(m *testing.M) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	os.Exit(m.Run())
}

// TestForwardSchedule: the body goes to the op as is, and the reply comes back as is, with the status of its error.
func TestForwardSchedule(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		reply  string
		status int
	}{
		{`{"schedule":{"id":"x"}}`, http.StatusOK},
		{`{"error":{"code":"invalid","message":"m"}}`, http.StatusBadRequest},
		{`{"error":{"code":"not_found","message":"m"}}`, http.StatusNotFound},
		{`{"error":{"code":"internal","message":"m"}}`, http.StatusInternalServerError},
	} {
		var op, payload string
		h := newHandler(func(o string, p []byte) ([]byte, error) {
			op, payload = o, string(p)

			return []byte(tc.reply), nil
		})
		w := serve(t, h, http.MethodPost, "/api/schedules/update", `{"id":"x"}`)
		if op != "update" || payload != `{"id":"x"}` {
			t.Errorf("forwarded %q %q, want update's body", op, payload)
		}
		if w.Code != tc.status || w.Body.String() != tc.reply {
			t.Errorf("%s: %d %q, want %d and the reply", tc.reply, w.Code, w.Body, tc.status)
		}
	}
}

// TestForwardSchedule_UnknownOp: an op that is not the scheduler's is not forwarded.
func TestForwardSchedule_UnknownOp(t *testing.T) {
	t.Parallel()

	h := newHandler(func(op string, _ []byte) ([]byte, error) {
		t.Errorf("%q forwarded", op)

		return nil, nil
	})
	for _, op := range []string{"x", "%2B", "%23"} {
		if w := serve(t, h, http.MethodPost, "/api/schedules/"+op, `{}`); w.Code != http.StatusNotFound ||
			decodeReply(t, w).Error.Code != "not_found" {
			t.Errorf("%s: %d %q, want not_found", op, w.Code, w.Body)
		}
	}
}

// TestForwardSchedule_Unavailable: a scheduler that cannot be reached is a 503, or a 504 once it timed out.
func TestForwardSchedule_Unavailable(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{mqtt.NotConnectedError{}, http.StatusServiceUnavailable, "unavailable"},
		{mqtt.TimeoutError{Op: "list"}, http.StatusGatewayTimeout, "timeout"},
	} {
		h := newHandler(func(string, []byte) ([]byte, error) { return nil, tc.err })
		if w := serve(t, h, http.MethodPost, "/api/schedules/list", `{}`); w.Code != tc.status ||
			decodeReply(t, w).Error.Code != tc.code {
			t.Errorf("%v: %d %q, want %d %s", tc.err, w.Code, w.Body, tc.status, tc.code)
		}
	}
}

// TestListOccurrence: each enabled schedule's times, merged into spans when close, a schedule with too many truncated.
func TestListOccurrence(t *testing.T) {
	t.Parallel()

	ids := [4]uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	h := listing(t, []schedule.Schedule{
		{ID: ids[0], Rule: hourly, Action: "noop", Enabled: true},
		{ID: ids[1], Rule: minutely, Action: "noop", Enabled: true},
		{ID: ids[2], Rule: daily, Action: "noop"},
		{ID: ids[3], Rule: every100ms, Action: "noop", Enabled: true},
	})
	w := serve(t, h, http.MethodGet, "/api/occurrences?"+day, "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %q, want 200", w.Code, w.Body)
	}
	got := decodeReply(t, w).Occurrences
	if len(got) != 3 || got[0].ID != ids[0] || got[1].ID != ids[1] || got[2].ID != ids[3] {
		t.Fatalf("occurrences %+v, want the enabled schedules'", got)
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if o := got[0]; len(o.Spans) != 24 || o.Truncated || !o.Next.After(time.Now()) {
		t.Errorf("hourly: %+v, want 24 times and a next one", o)
	} else {
		for i, s := range o.Spans {
			if at := start.Add(time.Duration(i) * time.Hour); !s.Start.Equal(at) || !s.End.Equal(at) || s.Count != 1 {
				t.Errorf("hourly: span %d %+v, want %s alone", i, s, at)
			}
		}
	}

	nine := start.Add(9 * time.Hour)
	if o := got[1]; len(o.Spans) != 1 || o.Truncated || !o.Next.IsZero() ||
		!o.Spans[0].Start.Equal(nine) || !o.Spans[0].End.Equal(nine.Add(59*time.Minute)) || o.Spans[0].Count != 60 {
		t.Errorf("minutely: %+v, want a span of 60 times from 09:00 to 09:59, and none next", o)
	}

	if o := got[2]; len(o.Spans) != 1 || !o.Truncated || o.Spans[0].Count != budget ||
		!o.Spans[0].Start.Equal(start) || !o.Spans[0].End.Equal(start.Add(24*time.Hour)) {
		t.Errorf("every 100ms: %+v, want a span truncated to the end of the day", o)
	}
}

// TestListOccurrence_Merge: times as far apart as the query's merge are merged.
func TestListOccurrence_Merge(t *testing.T) {
	t.Parallel()

	h := listing(t, []schedule.Schedule{{ID: uuid.New(), Rule: hourly, Action: "noop", Enabled: true}})
	w := serve(t, h, http.MethodGet, "/api/occurrences?"+day+"&merge=1h", "")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := decodeReply(t, w).Occurrences; len(got) != 1 || len(got[0].Spans) != 1 || got[0].Spans[0].Count != 24 ||
		!got[0].Spans[0].Start.Equal(start) || !got[0].Spans[0].End.Equal(start.Add(23*time.Hour)) {
		t.Errorf("%d %q, want a span of 24 times", w.Code, w.Body)
	}
}

// TestListOccurrence_Invalid: a range that does not parse, ends before it starts or is too long, or a merge that does
// not parse or is out of bounds, is refused before the scheduler is asked.
func TestListOccurrence_Invalid(t *testing.T) {
	t.Parallel()

	h := newHandler(func(op string, _ []byte) ([]byte, error) {
		t.Errorf("%q forwarded", op)

		return nil, nil
	})
	for _, q := range []string{
		"from=2026-01-01&to=2026-01-02T00:00:00Z",
		"from=2026-01-02T00:00:00Z&to=2026-01-01T00:00:00Z",
		"from=2026-01-01T00:00:00Z&to=2026-04-01T00:00:00Z",
		day + "&merge=-1s",
		day + "&merge=25h",
		day + "&merge=x",
	} {
		if w := serve(t, h, http.MethodGet, "/api/occurrences?"+q, ""); w.Code != http.StatusBadRequest ||
			decodeReply(t, w).Error.Code != "invalid" {
			t.Errorf("%s: %d %q, want invalid", q, w.Code, w.Body)
		}
	}
}

// TestListOccurrence_Refused: the scheduler's refusal of the list is relayed.
func TestListOccurrence_Refused(t *testing.T) {
	t.Parallel()

	h := newHandler(func(string, []byte) ([]byte, error) {
		return []byte(`{"error":{"code":"internal","message":"store down"}}`), nil
	})
	w := serve(t, h, http.MethodGet, "/api/occurrences?"+day, "")
	if r := decodeReply(t, w); w.Code != http.StatusInternalServerError || r.Error.Code != "internal" {
		t.Errorf("%d %q, want internal", w.Code, w.Body)
	}
}

// TestPreviewRule: the next times of a rule after now, without asking the scheduler.
func TestPreviewRule(t *testing.T) {
	t.Parallel()

	h := newHandler(nil)
	w := serve(t, h, http.MethodPost, "/api/preview", `{"rule":"`+jsonLine(hourly)+`"}`)
	times := decodeReply(t, w).Times
	if w.Code != http.StatusOK || len(times) != previews {
		t.Fatalf("%d %q, want %d times", w.Code, w.Body, previews)
	}
	if now := time.Now(); !times[0].After(now) || times[0].After(now.Add(time.Hour)) {
		t.Errorf("first time %s, want within the hour after %s", times[0], now)
	}
	for i := 1; i < len(times); i++ {
		if d := times[i].Sub(times[i-1]); d != time.Hour {
			t.Errorf("times %d and %d %s apart, want an hour", i-1, i, d)
		}
	}

	// A rule ended has none.
	w = serve(t, h, http.MethodPost, "/api/preview", `{"rule":"`+jsonLine(minutely)+`"}`)
	if r := decodeReply(t, w); w.Code != http.StatusOK || r.Times == nil || len(r.Times) != 0 {
		t.Errorf("ended: %d %q, want []", w.Code, w.Body)
	}
}

// TestPreviewRule_Invalid: a rule rrule refuses, or a body that does not decode, is invalid.
func TestPreviewRule_Invalid(t *testing.T) {
	t.Parallel()

	h := newHandler(nil)
	for _, body := range []string{
		`{"rule":"RRULE:FREQ=DAILY"}`,
		`{"rule":"DTSTART:20260101T000000Z\nRRULE:FREQ=DAILY;BYDAY=XX"}`,
		`{"rule":"` + jsonLine(hourly) + `","action":"noop"}`,
		`{`,
	} {
		if w := serve(t, h, http.MethodPost, "/api/preview", body); w.Code != http.StatusBadRequest ||
			decodeReply(t, w).Error.Code != "invalid" {
			t.Errorf("%s: %d %q, want invalid", body, w.Code, w.Body)
		}
	}
}

// TestRoutes_CrossOrigin: another site cannot have a browser change schedules.
func TestRoutes_CrossOrigin(t *testing.T) {
	t.Parallel()

	h := newHandler(func(op string, _ []byte) ([]byte, error) {
		t.Errorf("%q forwarded", op)

		return nil, nil
	})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/schedules/delete", strings.NewReader(`{}`))
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	h.routes().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("%d, want 403", w.Code)
	}
}

// TestRoutes_Static: the client is served at /.
func TestRoutes_Static(t *testing.T) {
	t.Parallel()

	h := newHandler(nil)
	if w := serve(t, h, http.MethodGet, "/", ""); w.Code != http.StatusOK || w.Body.String() != "<html></html>" {
		t.Errorf("%d %q, want index.html", w.Code, w.Body)
	}
}

// newHandler returns a handler whose scheduler replies with f, serving a client of an index.html.
func newHandler(f fake) *handler {
	return &handler{scheduler: f, static: fstest.MapFS{"index.html": {Data: []byte("<html></html>")}}}
}

// listing returns a handler whose scheduler lists schedules.
func listing(t *testing.T, schedules []schedule.Schedule) *handler {
	t.Helper()

	list, err := json.Marshal(map[string]any{"schedules": schedules})
	if err != nil {
		t.Fatal(err)
	}

	return newHandler(func(op string, _ []byte) ([]byte, error) {
		if op != "list" {
			t.Errorf("%q forwarded, want list", op)
		}

		return list, nil
	})
}

// serve serves a request of method on target with body, from the same origin, and returns its response.
func serve(t *testing.T, h *handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w := httptest.NewRecorder()
	h.routes().ServeHTTP(w, r)

	return w
}

// decodeReply returns w's body, which must be JSON.
func decodeReply(t *testing.T, w *httptest.ResponseRecorder) reply {
	t.Helper()

	var r reply
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("%q: %v", w.Body, err)
	}
	if r.Error == nil {
		r.Error = &responseError{}
	}

	return r
}

// jsonLine escapes the line feeds of a rule for a JSON string.
func jsonLine(rule string) string {
	return strings.ReplaceAll(rule, "\n", `\n`)
}

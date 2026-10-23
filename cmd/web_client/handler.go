package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/elojah/scheduler/internal/schedule"
	"github.com/elojah/scheduler/pkg/mqtt"
)

// maxBody bounds a request's body.
const maxBody = 1 << 20

// requester sends a request to the scheduler and returns its reply: an mqtt.Client.
type requester interface {
	Request(ctx context.Context, op string, payload []byte) ([]byte, error)
}

// handler serves the client, and the API's endpoints, one method each.
type handler struct {
	scheduler requester
	// static is the built client.
	static fs.FS
}

type response struct {
	Occurrences []occurrence   `json:"occurrences,omitzero"`
	Times       []time.Time    `json:"times,omitzero"`
	Error       *responseError `json:"error,omitempty"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// routes returns the handler of every route.
func (h *handler) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/schedules/{op}", h.ForwardSchedule)
	mux.HandleFunc("GET /api/occurrences", h.ListOccurrence)
	mux.HandleFunc("POST /api/preview", h.PreviewRule)
	mux.Handle("GET /", http.FileServerFS(h.static))

	// The API has no auth: no other site may have a browser change schedules.
	return http.NewCrossOriginProtection().Handler(mux)
}

// list returns the scheduler's schedules.
func (h *handler) list(ctx context.Context) ([]schedule.Schedule, error) {
	reply, err := h.scheduler.Request(ctx, "list", []byte(`{}`))
	if err != nil {
		return nil, err
	}
	var res struct {
		Schedules []schedule.Schedule `json:"schedules"`
		Error     *SchedulerError     `json:"error"`
	}
	if err := json.Unmarshal(reply, &res); err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, *res.Error
	}

	return res.Schedules, nil
}

// decode reads body into v, refusing unknown fields, as the scheduler does.
func decode(body io.Reader, v any) error {
	d := json.NewDecoder(body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return InvalidRequestError{Err: err}
	}

	return nil
}

// write writes res as JSON, with status.
func write(w http.ResponseWriter, status int, res response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		log.Error().Err(err).Msg("response not written")
	}
}

// fail writes the response refusing a request with err.
func fail(w http.ResponseWriter, err error) {
	code := "internal"
	var se SchedulerError
	switch {
	case errors.As(err, new(InvalidRequestError)),
		errors.As(err, new(InvalidRangeError)),
		errors.As(err, new(InvalidMergeError)),
		errors.As(err, new(InvalidRuleError)):
		code = "invalid"
	case errors.As(err, new(UnknownOpError)):
		code = "not_found"
	case errors.As(err, new(mqtt.NotConnectedError)):
		code = "unavailable"
	case errors.As(err, new(mqtt.TimeoutError)):
		code = "timeout"
	case errors.As(err, &se):
		code = se.Code
	}
	write(w, status(code), response{Error: &responseError{Code: code, Message: err.Error()}})
}

// status returns the HTTP status of an error's code, the scheduler's or the web client's.
func status(code string) int {
	switch code {
	case "invalid":
		return http.StatusBadRequest
	case "not_found":
		return http.StatusNotFound
	case "unavailable":
		return http.StatusServiceUnavailable
	case "timeout":
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

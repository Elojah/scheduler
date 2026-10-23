package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/elojah/scheduler/internal/schedule"
)

// handler serves the API's ops, one method each.
type handler struct {
	schedule schedule.Service
}

type request struct {
	ID uuid.UUID `json:"id"`
}

type response struct {
	Schedule  *schedule.Schedule  `json:"schedule,omitempty"`
	Schedules []schedule.Schedule `json:"schedules,omitzero"`
	Error     *responseError      `json:"error,omitempty"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// serve runs op with payload, and returns the reply: an mqtt.Handler.
func (h *handler) serve(ctx context.Context, op string, payload []byte) []byte {
	var (
		res response
		err error
	)
	switch op {
	case "create":
		res, err = h.CreateSchedule(ctx, payload)
	case "get":
		res, err = h.GetSchedule(ctx, payload)
	case "list":
		res, err = h.ListSchedule(ctx, payload)
	case "update":
		res, err = h.UpdateSchedule(ctx, payload)
	case "delete":
		res, err = h.DeleteSchedule(ctx, payload)
	default:
		err = UnknownOpError{Op: op}
		log.Error().Err(err).Msg("unknown op")
	}
	if err != nil {
		res = failure(err)
	}

	reply, err := json.Marshal(res)
	if err != nil {
		log.Error().Err(err).Str("op", op).Msg("reply not encoded")

		return nil
	}

	return reply
}

// decode reads payload into v, refusing unknown fields, so that a misspelt one is not silently left out.
func decode(payload []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(payload))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return InvalidRequestError{Err: err}
	}

	return nil
}

// failure returns the response refusing a request with err: invalid, not_found, or internal for anything else.
func failure(err error) response {
	code := "internal"
	switch {
	case errors.As(err, new(InvalidRequestError)),
		errors.As(err, new(UnknownOpError)),
		errors.As(err, new(schedule.InvalidRRuleError)),
		errors.As(err, new(schedule.UnknownOperationError)),
		errors.As(err, new(schedule.NegativeDurationError)),
		errors.As(err, new(schedule.InvalidParamsError)):
		code = "invalid"
	case errors.As(err, new(schedule.NotFoundError)):
		code = "not_found"
	}

	return response{Error: &responseError{Code: code, Message: err.Error()}}
}

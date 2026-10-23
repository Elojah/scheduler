package main

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"

	"github.com/rs/zerolog/log"
)

// ops are the scheduler's. No other is forwarded: a + or a # would make an invalid topic, which the broker drops the
// connection for.
var ops = []string{"create", "get", "list", "update", "delete"}

// ForwardSchedule sends the body to the scheduler's op, and writes its reply as is, with the status of its error, if
// any.
func (h *handler) ForwardSchedule(w http.ResponseWriter, r *http.Request) {
	logger := log.With().Str("method", "forward_schedule").Logger()

	// #MARK:Check request
	op := r.PathValue("op")
	if !slices.Contains(ops, op) {
		err := UnknownOpError{Op: op}
		logger.Error().Err(err).Msg("invalid request")
		fail(w, err)

		return
	}
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		err = InvalidRequestError{Err: err}
		logger.Error().Err(err).Msg("invalid request")
		fail(w, err)

		return
	}

	// #MARK:Forward request
	reply, err := h.scheduler.Request(r.Context(), op, payload)
	if err != nil {
		logger.Error().Err(err).Str("op", op).Msg("failed to forward request")
		fail(w, err)

		return
	}
	var res struct {
		Error *SchedulerError `json:"error"`
	}
	if err := json.Unmarshal(reply, &res); err != nil {
		logger.Error().Err(err).Str("op", op).Msg("invalid reply")
		fail(w, err)

		return
	}

	code := http.StatusOK
	if res.Error != nil {
		code = status(res.Error.Code)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	if _, err := w.Write(reply); err != nil { //nolint:gosec // JSON, as decoded above, never sniffed as HTML
		logger.Error().Err(err).Msg("response not written")

		return
	}

	logger.Info().Str("op", op).Int("status", code).Msg("request completed")
}

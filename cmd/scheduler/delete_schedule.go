package main

import (
	"context"

	"github.com/rs/zerolog/log"
)

// DeleteSchedule removes the schedule of the payload's id, and stops its alarm.
func (h *handler) DeleteSchedule(ctx context.Context, payload []byte) (response, error) {
	logger := log.With().Str("method", "delete_schedule").Logger()

	// #MARK:Check request
	var req request
	if err := decode(payload, &req); err != nil {
		logger.Error().Err(err).Msg("invalid request")

		return response{}, err
	}

	// #MARK:Delete schedule
	if err := h.schedule.Delete(ctx, req.ID); err != nil {
		logger.Error().Err(err).Stringer("id", req.ID).Msg("failed to delete schedule")

		return response{}, err
	}

	logger.Info().Stringer("id", req.ID).Msg("request completed")

	return response{}, nil
}

package main

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/elojah/scheduler/internal/schedule"
)

// UpdateSchedule replaces the schedule of the payload's id with the payload.
func (h *handler) UpdateSchedule(ctx context.Context, payload []byte) (response, error) {
	logger := log.With().Str("method", "update_schedule").Logger()

	// #MARK:Check request
	var req schedule.Schedule
	if err := decode(payload, &req); err != nil {
		logger.Error().Err(err).Msg("invalid request")

		return response{}, err
	}

	// #MARK:Update schedule
	sc, err := h.schedule.Update(ctx, req)
	if err != nil {
		logger.Error().Err(err).Stringer("id", req.ID).Msg("failed to update schedule")

		return response{}, err
	}

	logger.Info().Stringer("id", sc.ID).Msg("request completed")

	return response{Schedule: &sc}, nil
}

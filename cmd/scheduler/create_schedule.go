package main

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/elojah/scheduler/internal/schedule"
)

// CreateSchedule stores the schedule of payload under a new ID, and rings it if enabled.
func (h *handler) CreateSchedule(ctx context.Context, payload []byte) (response, error) {
	logger := log.With().Str("method", "create_schedule").Logger()

	// #MARK:Check request
	var req schedule.Schedule
	if err := decode(payload, &req); err != nil {
		logger.Error().Err(err).Msg("invalid request")

		return response{}, err
	}

	// #MARK:Create schedule
	sc, err := h.schedule.Create(ctx, req)
	if err != nil {
		logger.Error().Err(err).Msg("failed to create schedule")

		return response{}, err
	}

	logger.Info().Stringer("id", sc.ID).Msg("request completed")

	return response{Schedule: &sc}, nil
}

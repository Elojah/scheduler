package main

import (
	"context"

	"github.com/rs/zerolog/log"
)

// GetSchedule returns the schedule of the payload's id.
func (h *handler) GetSchedule(ctx context.Context, payload []byte) (response, error) {
	logger := log.With().Str("method", "get_schedule").Logger()

	// #MARK:Check request
	var req request
	if err := decode(payload, &req); err != nil {
		logger.Error().Err(err).Msg("invalid request")

		return response{}, err
	}

	// #MARK:Get schedule
	sc, err := h.schedule.Get(ctx, req.ID)
	if err != nil {
		logger.Error().Err(err).Stringer("id", req.ID).Msg("failed to get schedule")

		return response{}, err
	}

	logger.Info().Stringer("id", sc.ID).Msg("request completed")

	return response{Schedule: &sc}, nil
}

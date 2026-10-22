package main

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/elojah/scheduler/internal/schedule"
)

// ListSchedule returns every schedule, in the order they were created. Its payload is ignored.
func (h *handler) ListSchedule(ctx context.Context, _ []byte) (response, error) {
	logger := log.With().Str("method", "list_schedule").Logger()

	// #MARK:List schedules
	schedules, err := h.schedule.List(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to list schedules")

		return response{}, err
	}
	if schedules == nil {
		// [] rather than nothing.
		schedules = []schedule.Schedule{}
	}

	logger.Info().Int("count", len(schedules)).Msg("request completed")

	return response{Schedules: schedules}, nil
}

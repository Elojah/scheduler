package main

import (
	"net/http"
	"time"

	"github.com/elojah/rrule"
	"github.com/rs/zerolog/log"
)

// previews is how many times a preview returns.
const previews = 5

type previewRequest struct {
	Rule string `json:"rule"`
}

// PreviewRule returns the next times of a rule after now, as the scheduler would run them, or why it would refuse it:
// it reads it with rrule.ParseSet, as schedule.Schedule.Parse does.
func (h *handler) PreviewRule(w http.ResponseWriter, r *http.Request) {
	logger := log.With().Str("method", "preview_rule").Logger()

	// #MARK:Check request
	var req previewRequest
	if err := decode(http.MaxBytesReader(w, r.Body, maxBody), &req); err != nil {
		logger.Error().Err(err).Msg("invalid request")
		fail(w, err)

		return
	}

	// #MARK:Parse rule
	set, err := rrule.ParseSet(req.Rule)
	if err != nil {
		// A rule being typed: not an error of the web client's.
		fail(w, InvalidRuleError{Err: err})

		return
	}

	// #MARK:Expand rule
	times := make([]time.Time, 0, previews)
	for t := time.Now(); len(times) < previews; {
		next, ok := set.After(t)
		if !ok {
			break
		}
		times = append(times, next)
		t = next
	}

	write(w, http.StatusOK, response{Times: times})
}

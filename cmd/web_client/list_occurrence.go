package main

import (
	"net/http"
	"net/url"
	"time"

	"github.com/elojah/rrule"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	// maxRange bounds a range of occurrences: a month, with the weeks it starts and ends in.
	maxRange = 62 * 24 * time.Hour
	// defaultMerge is the gap up to which times are merged into a span, unless the query says: closer, a week's
	// calendar could not tell them apart.
	defaultMerge = 15 * time.Minute
	// maxMerge bounds the query's merge.
	maxMerge = 24 * time.Hour
	// budget bounds how many times of a schedule are walked: past it, its last span is taken to last until the
	// range's end.
	budget = 10000
)

// occurrence holds a schedule's times in a range.
type occurrence struct {
	ID    uuid.UUID `json:"id"`
	Spans []span    `json:"spans"`
	// Next is its first time after now, if any.
	Next time.Time `json:"next,omitzero"`
	// Truncated is whether more than budget times were in the range, the last span ending at the range's end.
	Truncated bool `json:"truncated,omitzero"`
}

// span holds times of a schedule, each at most the query's merge after the previous: a single time if Count is 1.
type span struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Count int       `json:"count"`
}

// occurrenceQuery is what ListOccurrence's query holds.
type occurrenceQuery struct {
	// From and To bound the times, To excluded, RFC 3339 in the query.
	From, To time.Time
	// Merge is the gap up to which times are merged into a span, a Go duration in the query: a month's calendar
	// takes a longer one than a week's.
	Merge time.Duration
}

// ListOccurrence returns the times of each enabled schedule from the query's from to its to, excluded, merged into
// spans.
func (h *handler) ListOccurrence(w http.ResponseWriter, r *http.Request) {
	logger := log.With().Str("method", "list_occurrence").Logger()

	// #MARK:Check request
	q, err := parseQuery(r.URL.Query())
	if err != nil {
		logger.Error().Err(err).Msg("invalid request")
		fail(w, err)

		return
	}

	// #MARK:List schedules
	schedules, err := h.list(r.Context())
	if err != nil {
		logger.Error().Err(err).Msg("failed to list schedules")
		fail(w, err)

		return
	}

	// #MARK:Expand rules
	now := time.Now()
	occurrences := []occurrence{}
	for _, sc := range schedules {
		if !sc.Enabled {
			continue
		}
		set, err := rrule.ParseSet(sc.Rule)
		if err != nil {
			// The scheduler refuses such a rule: it does not run.
			logger.Warn().Err(err).Stringer("id", sc.ID).Msg("invalid rule")

			continue
		}
		o := expand(set, q)
		o.ID = sc.ID
		if next, ok := set.After(now); ok {
			o.Next = next
		}
		occurrences = append(occurrences, o)
	}

	logger.Info().Int("count", len(occurrences)).Msg("request completed")

	write(w, http.StatusOK, response{Occurrences: occurrences})
}

// parseQuery returns what v holds.
func parseQuery(v url.Values) (occurrenceQuery, error) {
	var (
		q   = occurrenceQuery{Merge: defaultMerge}
		err error
	)
	if q.From, err = time.Parse(time.RFC3339, v.Get("from")); err != nil {
		return occurrenceQuery{}, InvalidRequestError{Err: err}
	}
	if q.To, err = time.Parse(time.RFC3339, v.Get("to")); err != nil {
		return occurrenceQuery{}, InvalidRequestError{Err: err}
	}
	if !q.To.After(q.From) || q.To.Sub(q.From) > maxRange {
		return occurrenceQuery{}, InvalidRangeError{From: q.From, To: q.To}
	}
	if v.Has("merge") {
		if q.Merge, err = time.ParseDuration(v.Get("merge")); err != nil {
			return occurrenceQuery{}, InvalidRequestError{Err: err}
		}
		if q.Merge < 0 || q.Merge > maxMerge {
			return occurrenceQuery{}, InvalidMergeError{Merge: q.Merge}
		}
	}

	return q, nil
}

// expand returns set's times in [q.From, q.To), merged into spans.
func expand(set rrule.Set, q occurrenceQuery) occurrence {
	o := occurrence{Spans: []span{}}
	walked := 0
	for t := range set.Between(q.From, q.To) {
		if walked == budget {
			o.Spans[len(o.Spans)-1].End = q.To
			o.Truncated = true

			break
		}
		walked++
		if last := len(o.Spans) - 1; last >= 0 && t.Sub(o.Spans[last].End) <= q.Merge {
			o.Spans[last].End = t
			o.Spans[last].Count++

			continue
		}
		o.Spans = append(o.Spans, span{Start: t, End: t, Count: 1})
	}

	return o
}

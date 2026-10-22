// Package schedule holds the Schedule, what keeps them (Store) and what runs them (Service), whose implementation is
// in package service, on a Store of package sqlite.
package schedule

import (
	"context"
	"encoding/json"
	"time"

	"github.com/elojah/rrule"
	"github.com/google/uuid"
)

// Schedule runs Action at each time of Rule while Enabled.
type Schedule struct {
	// ID is a UUIDv7 that Create assigns, so that IDs sort by creation.
	ID uuid.UUID `json:"id"`
	// Rule is a recurrence set as RFC 5545 writes it, read by rrule.ParseSet: a DTSTART line, then RRULE, RDATE,
	// EXDATE or EXRULE lines. It is kept as given.
	Rule string `json:"rule"`
	// Action is the key of the Action to run, in the Service's Actions.
	Action  string  `json:"action"`
	Options Options `json:"options"`
	Enabled bool    `json:"enabled"`
}

// Options tunes a Schedule's alarm, and holds its action's params. A zero duration leaves the alarm's default.
type Options struct {
	// MaxDelay drops a time passed by more than it, rather than running the action late (middleware.MaxDelay).
	MaxDelay Duration `json:"max_delay,omitzero"`
	// Coalesce runs the action only for the latest of a run of times passed by more than it (alarm.WithCoalesce).
	Coalesce Duration `json:"coalesce,omitzero"`
	// Spin busy-waits the last of it before each time, to the microsecond rather than the millisecond, taking a core
	// meanwhile (holder.NewSpin).
	Spin Duration `json:"spin,omitzero"`
	// Poll bounds a wait, so that a clock step or a suspend shows within it: a minute by default (alarm.WithPoll).
	Poll Duration `json:"poll,omitzero"`
	// Params is handed to the action as is.
	Params json.RawMessage `json:"params,omitempty"`
}

// Duration is a time.Duration written as time.Duration.String writes it, such as "1m30s", and read by
// time.ParseDuration.
type Duration time.Duration

// Store keeps schedules. Get, Update and Delete return a NotFoundError for an ID it does not hold.
//
//go:generate moq -rm -out store_moq.go . Store
type Store interface {
	Create(ctx context.Context, s Schedule) error
	Get(ctx context.Context, id uuid.UUID) (Schedule, error)
	List(ctx context.Context) ([]Schedule, error)
	Update(ctx context.Context, s Schedule) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// Service keeps schedules in a Store, and rings an alarm per enabled one. Create and Update return an error of this
// package if the schedule cannot run, and Get, Update and Delete a NotFoundError for an ID the Store does not hold.
//
//go:generate moq -rm -out service_moq.go . Service
type Service interface {
	Create(ctx context.Context, s Schedule) (Schedule, error)
	Get(ctx context.Context, id uuid.UUID) (Schedule, error)
	List(ctx context.Context) ([]Schedule, error)
	Update(ctx context.Context, s Schedule) (Schedule, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// MarshalText returns d as time.Duration.String writes it.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// UnmarshalText sets d to what time.ParseDuration reads from text.
func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = Duration(v)

	return nil
}

// SameTuning reports whether o and other build the same alarm: all but Params, which the alarm's action gets.
func (o Options) SameTuning(other Options) bool {
	return o.MaxDelay == other.MaxDelay && o.Coalesce == other.Coalesce && o.Spin == other.Spin && o.Poll == other.Poll
}

// Parse returns s's rule, or an InvalidRRuleError, UnknownActionError, NegativeDurationError or InvalidParamsError if
// s cannot run with actions.
func (s Schedule) Parse(actions Actions) (rrule.Set, error) {
	set, err := rrule.ParseSet(s.Rule)
	if err != nil {
		return rrule.Set{}, InvalidRRuleError{RRule: s.Rule, Err: err}
	}
	if _, ok := actions[s.Action]; !ok {
		return rrule.Set{}, UnknownActionError{Action: s.Action}
	}
	o := s.Options
	if min(o.MaxDelay, o.Coalesce, o.Spin, o.Poll) < 0 {
		return rrule.Set{}, NegativeDurationError{}
	}
	if len(o.Params) > 0 && !json.Valid(o.Params) {
		return rrule.Set{}, InvalidParamsError{}
	}

	return set, nil
}

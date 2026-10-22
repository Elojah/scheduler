package schedule

import (
	"context"
	"time"
)

// Action is run at each time of an enabled Schedule's rule, with the Schedule as it is then, in its alarm's goroutine:
// an Action slower than the gap between times makes the next ones late, which Options.MaxDelay and Options.Coalesce
// drop. ctx is cancelled once the Service's Close ctx is done.
type Action func(ctx context.Context, s Schedule, at time.Time) error

// Actions maps the keys that Schedule.Action names to what they run.
type Actions map[string]Action

// Blank returns placeholder actions, which do nothing: "noop".
func Blank() Actions {
	return Actions{
		"noop": func(context.Context, Schedule, time.Time) error { return nil },
	}
}

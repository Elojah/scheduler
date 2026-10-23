package schedule

import (
	"context"
	"time"
)

// Operation is run at each time of an enabled Schedule's rule, with the Schedule as of its last change of Operation or
// Params, in its alarm's goroutine: an Operation slower than the gap between times makes the next ones late, which
// Options.MaxDelay and Options.Coalesce drop. ctx is cancelled once the Service's Close ctx is done.
type Operation func(ctx context.Context, s Schedule, at time.Time) error

// Operations maps the keys that Schedule.Operation names to what they run.
type Operations map[string]Operation

// Blank returns placeholder operations, which do nothing: "noop".
func Blank() Operations {
	return Operations{
		"noop": func(context.Context, Schedule, time.Time) error { return nil },
	}
}

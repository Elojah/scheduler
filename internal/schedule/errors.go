package schedule

import (
	"strconv"

	"github.com/google/uuid"
)

// InvalidRRuleError refuses a Schedule whose Rule rrule.ParseSet refuses.
type InvalidRRuleError struct {
	RRule string
	Err   error
}

// Error implements error.
func (e InvalidRRuleError) Error() string {
	return "invalid rrule " + strconv.Quote(e.RRule) + ": " + e.Err.Error()
}

// Unwrap returns rrule's error.
func (e InvalidRRuleError) Unwrap() error {
	return e.Err
}

// UnknownOperationError refuses a Schedule whose Operation is not in the Service's Operations.
type UnknownOperationError struct {
	Operation string
}

// Error implements error.
func (e UnknownOperationError) Error() string {
	return "unknown operation " + strconv.Quote(e.Operation)
}

// NegativeDurationError refuses Options with a negative duration.
type NegativeDurationError struct{}

// Error implements error.
func (e NegativeDurationError) Error() string {
	return "negative duration"
}

// InvalidParamsError refuses Options whose Params are not JSON.
type InvalidParamsError struct{}

// Error implements error.
func (e InvalidParamsError) Error() string {
	return "params are not JSON"
}

// NotFoundError is returned for an ID the Store does not hold.
type NotFoundError struct {
	ID uuid.UUID
}

// Error implements error.
func (e NotFoundError) Error() string {
	return "schedule " + e.ID.String() + " not found"
}

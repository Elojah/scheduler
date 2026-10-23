package main

import (
	"strconv"
	"time"
)

// InvalidRequestError refuses a body or a query that does not decode: malformed JSON, an unknown field, or a time
// that is not RFC 3339.
type InvalidRequestError struct {
	Err error
}

// Error implements error.
func (e InvalidRequestError) Error() string {
	return "invalid request: " + e.Err.Error()
}

// Unwrap returns the decoder's error.
func (e InvalidRequestError) Unwrap() error {
	return e.Err
}

// InvalidRangeError refuses a range that ends before it starts, or is longer than maxRange.
type InvalidRangeError struct {
	From, To time.Time
}

// Error implements error.
func (e InvalidRangeError) Error() string {
	return "invalid range from " + e.From.Format(time.RFC3339) + " to " + e.To.Format(time.RFC3339) +
		": it must end after it starts, at most " + maxRange.String() + " later"
}

// InvalidMergeError refuses a merge that is negative, or longer than maxMerge.
type InvalidMergeError struct {
	Merge time.Duration
}

// Error implements error.
func (e InvalidMergeError) Error() string {
	return "invalid merge " + e.Merge.String() + ": it must be from 0 to " + maxMerge.String()
}

// InvalidRuleError refuses a rule that rrule.ParseSet refuses, as the scheduler does.
type InvalidRuleError struct {
	Err error
}

// Error implements error.
func (e InvalidRuleError) Error() string {
	return "invalid rule: " + e.Err.Error()
}

// Unwrap returns rrule's error.
func (e InvalidRuleError) Unwrap() error {
	return e.Err
}

// UnknownOpError refuses an op that is none of the scheduler's: it is not forwarded.
type UnknownOpError struct {
	Op string
}

// Error implements error.
func (e UnknownOpError) Error() string {
	return "unknown op " + strconv.Quote(e.Op)
}

// SchedulerError is the scheduler's refusal of a request.
type SchedulerError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error implements error.
func (e SchedulerError) Error() string {
	return "scheduler: " + e.Code + ": " + e.Message
}

package main

import "strconv"

// InvalidRequestError refuses a payload that does not decode: malformed JSON, or an unknown field.
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

// UnknownOpError refuses a request whose op is none of the API's.
type UnknownOpError struct {
	Op string
}

// Error implements error.
func (e UnknownOpError) Error() string {
	return "unknown op " + strconv.Quote(e.Op)
}

package sunspec

import "strconv"

// ParamsError refuses Params that do not decode: malformed JSON, an unknown field, or none.
type ParamsError struct {
	Err error
}

// Error implements error.
func (e ParamsError) Error() string {
	return "invalid sunspec params: " + e.Err.Error()
}

// Unwrap returns the decoder's error.
func (e ParamsError) Unwrap() error {
	return e.Err
}

// ModelNotFoundError is returned for a model the device at Address does not have.
type ModelNotFoundError struct {
	Address string
	Model   uint16
}

// Error implements error.
func (e ModelNotFoundError) Error() string {
	return "no model " + strconv.Itoa(int(e.Model)) + " at " + e.Address
}

// PointNotFoundError is returned for a point Model does not have.
type PointNotFoundError struct {
	Model uint16
	Point string
}

// Error implements error.
func (e PointNotFoundError) Error() string {
	return "model " + strconv.Itoa(int(e.Model)) + " has no point " + strconv.Quote(e.Point)
}

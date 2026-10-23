package mqtt

import "strconv"

// MissingClientIDError refuses a Config without a ClientID.
type MissingClientIDError struct{}

// Error implements error.
func (e MissingClientIDError) Error() string {
	return "mqtt: missing client id"
}

// NotConnectedError refuses a request while the Client is not subscribed to its replies: the broker is down, or the
// connection is coming up.
type NotConnectedError struct{}

// Error implements error.
func (e NotConnectedError) Error() string {
	return "mqtt: not connected"
}

// TimeoutError is a request that got no reply in time: the Service is down, or busy.
type TimeoutError struct {
	Op string
}

// Error implements error.
func (e TimeoutError) Error() string {
	return "mqtt: no reply to " + strconv.Quote(e.Op)
}

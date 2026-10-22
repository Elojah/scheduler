package mqtt

// MissingClientIDError refuses a Config without a ClientID.
type MissingClientIDError struct{}

// Error implements error.
func (e MissingClientIDError) Error() string {
	return "mqtt: missing client id"
}

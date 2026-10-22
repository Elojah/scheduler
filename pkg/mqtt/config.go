package mqtt

// Config is mqtt's config.
type Config struct {
	// Broker is the broker's URL, mqtt:// or tls://.
	Broker string `env:"MQTT_BROKER" env-default:"mqtt://localhost:1883" json:"broker"`
	// ClientID is required: the broker keeps the session under it across reconnections.
	ClientID string `env:"MQTT_CLIENT_ID" json:"client_id"`
	// Prefix is what comes before <Topic>/<op> in request topics, if any.
	Prefix string `env:"MQTT_PREFIX" json:"prefix"`
}

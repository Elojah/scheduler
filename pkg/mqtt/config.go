package mqtt

// Config is mqtt's config.
type Config struct {
	// Broker is the broker's URL, mqtt:// or tls://.
	Broker string `env:"MQTT_BROKER" env-default:"mqtt://localhost:1883" json:"broker"`
	// ClientID is required: the broker keeps the session under it across reconnections.
	ClientID string `env:"MQTT_CLIENT_ID" json:"client_id"`
	// Prefix is what comes before <Topic>/<op> in request topics, if any.
	Prefix string `env:"MQTT_PREFIX" json:"prefix"`
	// Workers is how many requests are served at once.
	Workers uint `env:"MQTT_WORKERS" env-default:"8" json:"workers"`
	// Queue is how many requests wait for a worker before the next are dropped.
	Queue uint `env:"MQTT_QUEUE" env-default:"64" json:"queue"`
	// ReplyTimeout bounds, in seconds, how long a reply or a SUBSCRIBE waits for the broker's acknowledgement.
	ReplyTimeout uint32 `env:"MQTT_REPLY_TIMEOUT" env-default:"10" json:"reply_timeout"`
	// KeepAlive is in seconds, 0 to disable it.
	KeepAlive uint16 `env:"MQTT_KEEP_ALIVE" env-default:"30" json:"keep_alive"`
}

package http

// Config is http's config.
type Config struct {
	// Address is the host and port to listen on: ":8080" on every interface.
	Address string `env:"HTTP_ADDRESS" env-default:":8080" json:"address"`
}

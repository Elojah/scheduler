package log

// Config is log's config.
type Config struct {
	// Level is what zerolog.ParseLevel reads: debug, info, warn or error.
	Level string `env:"LOG_LEVEL" env-default:"info" json:"level"`
}

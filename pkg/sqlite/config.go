package sqlite

// Config is sqlite's config.
type Config struct {
	// Path is the database's file, created if need be.
	Path string `env:"SQLITE_PATH" env-default:"scheduler.db" json:"path"`
}

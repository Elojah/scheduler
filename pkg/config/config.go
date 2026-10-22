// Package config reads a service's configuration from a JSON file and the environment.
package config

import (
	"os"

	"github.com/ilyakaznacheev/cleanenv"
)

// Populate reads cfg from filename, if it exists, then from the environment, which wins. A field still zero takes its
// env-default tag.
func Populate(cfg any, filename string) error {
	if filename != "" {
		if _, err := os.Stat(filename); err == nil {
			return cleanenv.ReadConfig(filename, cfg)
		}
	}

	return cleanenv.ReadEnv(cfg)
}

package main

import (
	tconfig "github.com/elojah/scheduler/pkg/config"
	"github.com/elojah/scheduler/pkg/log"
	"github.com/elojah/scheduler/pkg/mqtt"
	"github.com/elojah/scheduler/pkg/sqlite"
)

type config struct {
	Log    log.Config    `json:"log"`
	SQLite sqlite.Config `json:"sqlite"`
	MQTT   mqtt.Config   `json:"mqtt"`
}

// Populate reads c from filename, if it exists, then from the environment.
func (c *config) Populate(filename string) error {
	return tconfig.Populate(c, filename)
}

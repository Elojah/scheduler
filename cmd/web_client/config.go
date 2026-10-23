package main

import (
	tconfig "github.com/elojah/scheduler/pkg/config"
	thttp "github.com/elojah/scheduler/pkg/http"
	"github.com/elojah/scheduler/pkg/log"
	"github.com/elojah/scheduler/pkg/mqtt"
)

type config struct {
	Log  log.Config        `json:"log"`
	HTTP thttp.Config      `json:"http"`
	MQTT mqtt.ClientConfig `json:"mqtt"`
	Web  web               `json:"web"`
}

// web is the client's.
type web struct {
	// Static is the directory of the built client: cmd/client/dist.
	Static string `env:"WEB_STATIC" env-default:"/app/static" json:"static"`
}

// Populate reads c from filename, if it exists, then from the environment.
func (c *config) Populate(filename string) error {
	return tconfig.Populate(c, filename)
}

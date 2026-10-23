// Command web_client serves the scheduler's web UI: the client built in cmd/client, and an HTTP API forwarding to the
// scheduler over MQTT v5, until SIGHUP, SIGINT or SIGTERM. Its config is read from the JSON file its first argument
// names, if any, then from the environment.
//
//	POST /api/schedules/{op}  the scheduler's op, create, get, list, update or delete: its payload and its reply
//	GET  /api/occurrences     ?from=…&to=…[&merge=15m]: the times of each enabled schedule, merged into spans
//	POST /api/preview         {"rule": …}: the next times of a rule, or why the scheduler would refuse it
//
// A refused request gets {"error": {"code": …, "message": …}}, as the scheduler's, code being invalid (400),
// not_found (404), internal (500), or unavailable (503) and timeout (504) when the scheduler cannot be reached.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
	_ "time/tzdata" // TZIDs resolve in a scratch image

	"github.com/rs/zerolog/log"

	thttp "github.com/elojah/scheduler/pkg/http"
	tlog "github.com/elojah/scheduler/pkg/log"
	"github.com/elojah/scheduler/pkg/mqtt"
	"github.com/elojah/scheduler/pkg/shutdown"
)

const (
	// initTO bounds the init phase: config, listener and connection.
	initTO = 30 * time.Second
	// closeTO is how long the requests under way get once signalled.
	closeTO = 5 * time.Second
)

// version is set at build: -ldflags '-X main.version=…'.
var version string

func main() {
	var filename string
	if len(os.Args) >= 2 {
		filename = os.Args[1]
	}
	if err := run(os.Args[0], filename); err != nil {
		log.Error().Err(err).Msg("web_client stopped")
		os.Exit(1)
	}
}

// run dials the web client's parts and serves until signalled.
func run(prog string, filename string) error {
	ctx, cancel := context.WithTimeout(context.Background(), initTO)
	defer cancel()

	cfg := config{}
	if err := cfg.Populate(filename); err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var cs shutdown.Closers
	// fail closes what was dialed, err being why the rest was not.
	fail := func(err error) error {
		return errors.Join(err, cs.Close(ctx))
	}

	logs := tlog.Service{}
	if err := logs.Dial(ctx, cfg.Log); err != nil {
		return fmt.Errorf("dial log: %w", err)
	}
	cs = append(cs, &logs)
	log.Logger = logs.With().Str("version", version).Str("exe", prog).Logger()

	mqttc := mqtt.Client{Topic: "schedules"}
	if err := mqttc.Dial(ctx, cfg.MQTT); err != nil {
		return fail(fmt.Errorf("dial mqtt: %w", err))
	}
	cs = append(cs, &mqttc)

	static := os.DirFS(cfg.Web.Static)
	if _, err := fs.Stat(static, "index.html"); err != nil {
		// The API serves on: the client may be served by vite meanwhile.
		log.Warn().Err(err).Str("static", cfg.Web.Static).Msg("no client built: make client")
	}

	h := handler{scheduler: &mqttc, static: static}
	https := thttp.Service{Handler: h.routes()}
	if err := https.Dial(ctx, cfg.HTTP); err != nil {
		return fail(fmt.Errorf("dial http: %w", err))
	}
	cs = append(cs, &https)

	log.Info().Msg("web_client up")

	// Closed in reverse: requests stop, then the connection to the broker.
	return cs.WaitSignal(closeTO)
}

// Command scheduler keeps schedules in a SQLite database, runs the operation of each enabled one at each time of its
// rule, and serves them over MQTT v5 until SIGHUP, SIGINT or SIGTERM. Its config is read from the JSON file its first
// argument names, if any, then from the environment.
//
// A request is a PUBLISH on <prefix>/schedules/<op>, op being create, get, list, update or delete, with a JSON
// payload, and its reply goes to the request's Response Topic with its Correlation Data.
//
//	scheduler/v1/schedules/create  {"rule": …, "operation": …, "options": {…}, "enabled": true}  → {"schedule": {…}}
//	scheduler/v1/schedules/get     {"id": …}                                                     → {"schedule": {…}}
//	scheduler/v1/schedules/list    {}                                                            → {"schedules": […]}
//	scheduler/v1/schedules/update  {"id": …, "rule": …, "operation": …, …}                       → {"schedule": {…}}
//	scheduler/v1/schedules/delete  {"id": …}                                                     → {}
//
// A refused request gets {"error": {"code": …, "message": …}}, code being invalid, not_found or internal. A request
// without a Response Topic is handled, but gets no reply.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
	_ "time/tzdata" // TZIDs resolve on a gateway without a zoneinfo database

	"github.com/rs/zerolog/log"

	"github.com/elojah/scheduler/internal/schedule"
	"github.com/elojah/scheduler/internal/schedule/service"
	schedulesqlite "github.com/elojah/scheduler/internal/schedule/sqlite"
	tlog "github.com/elojah/scheduler/pkg/log"
	"github.com/elojah/scheduler/pkg/mqtt"
	"github.com/elojah/scheduler/pkg/shutdown"
	"github.com/elojah/scheduler/pkg/sqlite"
)

const (
	// initTO bounds the init phase: config, database, alarms and connection.
	initTO = 30 * time.Second
	// closeTO is how long the operations under way get once signalled, before their ctx is cancelled.
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
		log.Error().Err(err).Msg("scheduler stopped")
		os.Exit(1)
	}
}

// run dials the scheduler's parts and serves until signalled.
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

	sqlites := sqlite.Service{}
	if err := sqlites.Dial(ctx, cfg.SQLite); err != nil {
		return fail(fmt.Errorf("dial sqlite: %w", err))
	}
	cs = append(cs, &sqlites)

	store := &schedulesqlite.Store{Service: sqlites}
	if err := store.Init(ctx); err != nil {
		return fail(err)
	}

	scheduleService := &service.S{Store: store, Operations: schedule.Blank()}
	if err := scheduleService.Dial(ctx); err != nil {
		return fail(fmt.Errorf("dial schedule service: %w", err))
	}
	cs = append(cs, scheduleService)

	h := handler{schedule: scheduleService}
	mqtts := mqtt.Service{Topic: "schedules", Handler: h.serve}
	if err := mqtts.Dial(ctx, cfg.MQTT); err != nil {
		return fail(fmt.Errorf("dial mqtt: %w", err))
	}
	cs = append(cs, &mqtts)

	log.Info().Msg("scheduler up")

	// Closed in reverse: requests stop, then the alarms, then the database.
	return cs.WaitSignal(closeTO)
}

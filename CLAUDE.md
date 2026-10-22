# CLAUDE.md

`github.com/elojah/scheduler`, a service gluing `../rrule` and `../alarm`: Schedules kept in SQLite, an alarm per
enabled one running its action, CRUD over MQTT v5. Go 1.26.1. Laid out and run as `../trax`. Behaviour is in godoc and
the README, plans in TODO.md.

## Layout

- `cmd/scheduler/`: the binary. `main.go` dials the parts into `shutdown.Closers`, `config.go` the config,
  `handler.go` the API's dispatch, types and error mapping, one `<op>_schedule.go` per op, `errors.go` its errors. It
  embeds `time/tzdata`, so that TZIDs resolve on a bare gateway.
- `internal/schedule/`: the domain. `Schedule`, `Options`, `Duration`, `Parse` (validation), the `Store` and `Service`
  interfaces, `Action`/`Actions`/`Blank`, the typed errors, and their moq mocks (`*_moq.go`, generated and committed).
- `internal/schedule/service/`: `service.S` (the Service) and `runner.go` (an alarm per schedule).
- `internal/schedule/sqlite/`: the `Store`, embedding a `pkg/sqlite.Service`, schema in `schema.sql`.
- `pkg/`: libraries with no knowledge of schedules. `config` (cleanenv), `log` (zerolog), `shutdown` (Closers),
  `sqlite` (modernc.org/sqlite, pure Go, so that `CGO_ENABLED=0` builds), and `mqtt` (request/response over
  paho.golang's autopaho).
- Imports go `cmd` → `internal`, `pkg`. `pkg` never imports `internal`.

## Commands

```sh
go test -race ./...  # always -race
make check           # golangci-lint + tests (-shuffle=on -cpu 1,4)
make build           # bin/scheduler, CGO_ENABLED=0, -X main.version
make run             # CONFIG=config/scheduler/local.json, broker at localhost:1883
make generate        # moq mocks, after an interface in internal/ changed (moq in ~/go/bin)
```

Lint (`.golangci.yml`, alarm's) is `default: all` minus a disable list, tests included. Mind `nlreturn`,
`paralleltest` (`t.Parallel()` first), `err113`, `godot`, `funcorder` and `containedctx`: no struct holds a ctx.

## Conventions

- Comments are short: invariants, non-obvious whys, the public contract.
- A part is a struct set up by `Dial(ctx[, cfg Config])` and stopped by `Close(ctx)`, a `shutdown.Closer`; `main`
  appends it to `Closers`, which close in reverse. A `pkg` Config has `env`, `env-default` and `json` tags. No
  functional options.
- Logs go through zerolog's global `log`; handlers log with `log.With().Str("method", …)`.
- Errors are typed structs, returned as values. The API maps them with `errors.As`: `invalid` for
  `schedule.InvalidRRuleError`, `UnknownActionError`, `NegativeDurationError`, `InvalidParamsError` and
  `InvalidRequestError`, `UnknownOpError`; `not_found` for `schedule.NotFoundError`; anything else is `internal`.
- The API is strict, as rrule's parsing: unknown JSON fields are `invalid`.

## Invariants

- `service.S` writes the store before touching the runner, under `S.mu`, so that a refused change leaves the alarms as
  they were, and the alarms run what the store holds.
- `runner.run` Resets the alarm under way if its tuning (MaxDelay, Coalesce, Spin, Poll) is unchanged, storing the
  new Schedule in `ringing.schedule` first, which each time loads, so that a new Action or Params applies from the
  next time. Otherwise, or once Reset returns false (the old rule ended), it closes the alarm and starts another.
- Ring's ctx is the caller's `WithoutCancel`: a request's ctx never stops an alarm. Only `stop` (Close) and
  `shutdown` do. `stop` never waits, so a request is never blocked by an action.
- `alarms` holds each schedule's current alarm, `live` every Ring not returned, replaced and stopped ones included,
  so that `shutdown` cancels them all once its ctx is done. A Ring removes itself from both when it returns.
- `closed` keeps alarms from starting after `shutdown`, whose `wg.Wait` would race a `wg.Go`.
- `pkg/mqtt`'s `OnPublishReceived` never blocks: it queues requests (64) for one loop, and drops them when full, so
  that paho reads on, the PUBACKs of the loop's replies included. One loop keeps requests in order. Its connection
  runs on a ctx detached from Dial's, which is the init timeout. Close waits for the request under way and its
  reply before it disconnects.
- `pkg/sqlite` holds one connection, and the Store binds `options` as text: a `[]byte` would be a BLOB, which a
  STRICT TEXT column refuses.

## Tests

- External test packages, except `cmd/scheduler` (`package main`, which testpackage allows). Name a test after its
  method, with a suffix for a case: `TestUpdate_Reset`. Each package's `TestMain` silences zerolog.
- `service` tests run in a `synctest` bubble on an in-memory Store, with actions logging what they run, and `slow`
  when it ran too, which tells a Reset (the next time runs once the action returns) from a new alarm (it runs
  meanwhile): `TestUpdate_Reset` against `TestUpdate_Options`. `holder.Spin` never ends in a bubble: no test spins.
- `sqlite`, `pkg/mqtt` and `cmd/scheduler` run on the real clock. SQLite runs in `t.TempDir()`. mochi-mqtt runs in
  process on `127.0.0.1:0`, with `await` polling its subscriptions until the Service subscribed. `cmd/scheduler`
  calls `handler.serve` directly, with no broker, and uses `schedule.ServiceMock` for `internal`.

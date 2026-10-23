# CLAUDE.md

`github.com/elojah/scheduler`, a service gluing `../rrule` and `../alarm`: Schedules kept in SQLite, an alarm per
enabled one running its action, CRUD over MQTT v5, and a web UI over it. Go 1.26.1. Laid out and run as `../trax`, its
client stripped down. Behaviour is in godoc and the README, plans in TODO.md.

## Layout

- `cmd/scheduler/`: the binary. `main.go` dials the parts into `shutdown.Closers`, `config.go` the config,
  `handler.go` the API's dispatch, types and error mapping, one `<op>_schedule.go` per op, `errors.go` its errors. It
  embeds `time/tzdata`, so that TZIDs resolve on a bare gateway.
- `cmd/web_client/`: the web UI's server, trax's `web_client` stripped down. `main.go` dials an `mqtt.Client` and a
  `pkg/http.Service`, `handler.go` the routes, the error envelope and the `requester` interface (an `mqtt.Client`),
  one file per endpoint: `forward_schedule.go` (an op, forwarded as is), `list_occurrence.go` (rules expanded by
  rrule, merged into spans), `preview_rule.go`. It embeds `time/tzdata` too.
- `cmd/client/`: the web UI, trax's client stripped down: Vue 3, Vite, TS, PrimeVue (Aura, trax's sky/amber),
  Tailwind, FullCalendar, one Pinia store (`stores/schedule.ts`). No router (two `Tabs`), auth, i18n, protobuf,
  pino, zod or eslint. `components/schedule/{Table,Dialog,Calendar}.vue`, `utils/{api,rule,date}.ts`. Prettier is
  trax's. `public/logo.svg` is a copy of `assets/logo.svg`.
- `internal/schedule/`: the domain. `Schedule`, `Options`, `Duration`, `Parse` (validation), the `Store` and `Service`
  interfaces, `Action`/`Actions`/`Blank`, the typed errors, and their moq mocks (`*_moq.go`, generated and committed).
- `internal/schedule/service/`: `service.S` (the Service) and `runner.go` (an alarm per schedule).
- `internal/schedule/sqlite/`: the `Store`, embedding a `pkg/sqlite.Service`, schema in `schema.sql`.
- `pkg/`: libraries with no knowledge of schedules. `config` (cleanenv), `log` (zerolog), `shutdown` (Closers),
  `sqlite` (modernc.org/sqlite, pure Go, so that `CGO_ENABLED=0` builds), `http` (a `net/http` server as a part,
  imported as `thttp`), and `mqtt` (request/response over paho.golang's autopaho: `Service` serves, `Client`
  requests).
- `docker/`: `scheduler/Dockerfile` (cross-compiling builder, `scratch` runner as uid 65532, the database in `/data`),
  `web_client/Dockerfile` (node builds the client, then the same) and `mosquitto/mosquitto.conf`, for
  `docker-compose.yml`, which mounts `config/{scheduler,web_client}/docker.json`.
- Imports go `cmd` → `internal`, `pkg`. `pkg` never imports `internal`.

## Commands

```sh
go test -race ./...  # always -race
make check           # golangci-lint + tests (-shuffle=on -cpu 1,4)
make build           # bin/scheduler, CGO_ENABLED=0, -X main.version
make run             # CONFIG=config/scheduler/local.json, broker at localhost:1883
make generate        # moq mocks, after an interface in internal/ changed (moq in ~/go/bin)
make client          # cmd/client/dist: npm ci, vue-tsc, vite build
make run-web_client  # CONFIG_WEB=config/web_client/local.json, on localhost:8080
cd cmd/client && npm run dev  # hot reload on :5173, /api proxied to :8080
cd cmd/client && npx prettier --check src/  # CI checks it
docker compose up -d --build  # mosquitto on localhost:1883, the scheduler, the web UI on :8080; `up -d mosquitto` for make run
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
- The web client's endpoints reply with the same envelope, adding `unavailable` (503, `mqtt.NotConnectedError`) and
  `timeout` (504, `mqtt.TimeoutError`). A forwarded reply passes through as is, its status from its `error.code`.

## Invariants

- A change of a schedule (Create, Update, Delete) holds its lock in `service.S` over the store write and the runner
  step, else the store and the alarm could each keep a different one (an Update racing a Delete leaves an alarm).
  `S.locks` is a `[256]sync.Mutex` picked by the ID's last byte, random in a UUIDv7: no map to grow or clean, and
  changes of other schedules run meanwhile, but for those sharing the lock. The runner has no lock of its own: it
  relies on that one for each schedule, and on `sync.Map`s across them. `S` writes the store before touching the
  runner, so that a refused change leaves the alarms as they were. Close runs once requests stopped, which `main`'s
  Closers order does.
- `runner.update` does only what changed: disabled, it stops the alarm; with none or a new tuning (MaxDelay,
  Coalesce, Spin, Poll), it starts another. Else a new Action or Params swaps `ringing.fn`, the callback the alarm
  loads at each time, and a new rule Resets the alarm, starting another once Reset returns false (the old rule
  ended). `ringing.schedule` is the last one applied, for update to diff: its tuning is the one `alarm.New` took.
- Ring's ctx is the caller's `WithoutCancel`: a request's ctx never stops an alarm. Only `stop` (Close) and
  `shutdown` do. `stop` never waits, so a request is never blocked by an action.
- `alarms` (a `sync.Map`) holds each enabled schedule's alarm, ended ones included until updated or deleted. `live`
  (a `sync.Map`, which a Ring leaves when it returns) maps every Ring not returned, replaced and stopped ones
  included, to its ctx's cancel, so that `shutdown` cancels them all once its ctx is done.
- `closed` (atomic) keeps alarms from starting after `shutdown`.
- `pkg/mqtt`'s `OnPublishReceived` never blocks: it queues requests (`Config.Queue`, 64) for `Config.Workers` (8),
  and drops them when full, so that paho reads on, the PUBACKs of the workers' replies included. Requests are served
  in no given order, the handler concurrently: `service.S`'s per-schedule locks keep changes of one schedule
  consistent. Its connection runs on a ctx detached from Dial's, which is the init timeout. Close waits for the
  requests under way and their replies before it disconnects.
- `pkg/mqtt`'s `Client` is ready once the SUBACK of `replies/<client_id>` came on the current connection: `conn`
  counts connections made and dropped, so that a late SUBACK of a dropped one is ignored. Until then, and after
  Close, `Request` returns a `NotConnectedError` at once rather than lose its reply. Replies go to buffered chans in a
  `sync.Map` keyed by the request's UUID, which `OnPublishReceived` never blocks on.
- `cmd/web_client` forwards only the scheduler's five ops: a `+` or `#` in a topic would make the broker drop its
  connection. `routes` wraps all in `http.CrossOriginProtection`: no auth, so no other site's POST.
- `/api/occurrences` walks at most `budget` (10 000) times per schedule and range (at most 62 days); past it, the last
  span lasts until the range's end. The calendar asks `merge=15m` for a day or week and `2h` for a month, telling
  them apart by a range above 8 days (a week is 7 days and an hour when clocks go back).
- `pkg/sqlite` holds one connection, and the Store binds `options` as text: a `[]byte` would be a BLOB, which a
  STRICT TEXT column refuses.

## Tests

- External test packages, except `cmd/scheduler` and `cmd/web_client` (`package main`, which testpackage allows). Name a test after its
  method, with a suffix for a case: `TestUpdate_Reset`. Each package's `TestMain` silences zerolog.
- `service` tests run in a `synctest` bubble on an in-memory Store, with actions logging what they run, and `slow`
  when it ran too, which tells a Reset (the next time runs once the action returns) from a new alarm (it runs
  meanwhile): `TestUpdate_Reset` against `TestUpdate_Options`. `holder.Spin` never ends in a bubble: no test spins.
- `cmd/web_client` tests `routes()` with `httptest` and a `fake` requester, a func: no broker.
- `cmd/client` has no tests: CI runs prettier, `vue-tsc` and `vite build`.
- `sqlite`, `pkg/mqtt` and `cmd/scheduler` run on the real clock. SQLite runs in `t.TempDir()`. mochi-mqtt runs in
  process on `127.0.0.1:0`, with `await` polling its subscriptions until the Service subscribed. `cmd/scheduler`
  calls `handler.serve` directly, with no broker, and uses `schedule.ServiceMock` for `internal`.

<p align="center">
  <img src="assets/logo.svg" alt="scheduler" width="180">
</p>

# scheduler

[![CI](https://github.com/Elojah/scheduler/actions/workflows/ci.yml/badge.svg)](https://github.com/Elojah/scheduler/actions/workflows/ci.yml)

Runs an action at each time of a recurrence rule, for schedules kept in SQLite and managed over MQTT v5. It glues
[rrule](https://github.com/elojah/rrule), which expands RFC 5545 rules, and [alarm](https://github.com/elojah/alarm),
which fires a func at each of their times, one alarm per schedule.

```sh
docker run --rm -p 1883:1883 eclipse-mosquitto mosquitto -c /mosquitto-no-auth.conf
go run ./cmd/scheduler config/scheduler/local.json
```

```sh
mosquitto_rr -V 5 -t scheduler/v1/schedules/create -e replies/me \
  -m '{"rule":"DTSTART:20260101T090000Z\nRRULE:FREQ=DAILY;BYDAY=MO,TU,WE,TH,FR","action":"noop","enabled":true}'
```

## Schedule

```json
{
  "id": "01a163b0-fa59-7463-86e9-6a1c8baea7bb",
  "rule": "DTSTART;TZID=Europe/Paris:20260101T090000\nRRULE:FREQ=DAILY",
  "action": "noop",
  "options": {"max_delay": "1s", "params": {"relay": 1}},
  "enabled": true
}
```

| Field | Holds |
| --- | --- |
| `id` | A UUIDv7, assigned by create: IDs sort by creation. |
| `rule` | A recurrence set, as RFC 5545 writes it: a `DTSTART` line, then `RRULE`, `RDATE`, `EXDATE` or `EXRULE` lines. Kept as sent. |
| `action` | The key of a predefined action. Only `noop` for now. |
| `options` | The alarm's tuning, and the action's params. |
| `enabled` | Whether the action runs. |

| Option | Does |
| --- | --- |
| `max_delay` | Drops a time passed by more than it, rather than running the action late. |
| `coalesce` | Of a run of times passed by more than it, runs the action only for the latest. |
| `spin` | Busy-waits the last of it before each time, to the microsecond rather than the millisecond, taking a core meanwhile. |
| `poll` | Bounds a wait, so that a clock step or a suspend shows within it: a minute by default. |
| `params` | Any JSON, handed to the action as is. |

Durations are Go's, such as `"250ms"` or `"1m30s"`.

## MQTT API

The scheduler is a client of the broker. A request is a PUBLISH, with a JSON payload, to `<prefix>/schedules/<op>`,
the prefix being `mqtt.prefix` (`scheduler/v1` in `config/scheduler/local.json`). Its reply goes to the request's Response Topic, with its
Correlation Data: MQTT v5's request/response. A request without a Response Topic is served, but gets no reply.

| Topic | Payload | Reply |
| --- | --- | --- |
| `scheduler/v1/schedules/create` | `{rule, action, options, enabled}` | `{"schedule": {…}}` |
| `scheduler/v1/schedules/get` | `{id}` | `{"schedule": {…}}` |
| `scheduler/v1/schedules/list` | `{}` | `{"schedules": […]}` |
| `scheduler/v1/schedules/update` | `{id, rule, action, options, enabled}` | `{"schedule": {…}}` |
| `scheduler/v1/schedules/delete` | `{id}` | `{}` |

A refused request gets `{"error": {"code": …, "message": …}}`:

| Code | When |
| --- | --- |
| `invalid` | Malformed JSON, an unknown field or op, a rule rrule refuses, an unknown action, a negative duration. |
| `not_found` | No schedule has the id. |
| `internal` | The database failed. |

## Behaviour worth knowing

- **Times start at now.** A schedule created, updated or loaded at startup runs from the next time of its rule:
  times passed while it was disabled, or the scheduler down, are not caught up.
- **Update does only what changed.** A new rule Resets the alarm, once the action under way returns. A change of
  `max_delay`, `coalesce`, `spin` or `poll` replaces the alarm instead, since an alarm's tuning is fixed when it is
  created. A new action or params applies from the next time, the alarm's times left as they were.
- **An action runs in its schedule's alarm goroutine**: a slow one makes the next times of its schedule late, not
  those of others. `max_delay` and `coalesce` drop such late times.
- **Disable and delete never wait**: the action under way finishes. On SIGHUP, SIGINT or SIGTERM, the request under
  way is replied to, then actions under way get 5s in all before their ctx is cancelled.
- **Alarms ring whether the broker is up or not**: the scheduler reconnects on its own.
- **`DTSTART` is required**: a bare `FREQ=…` has no time to start from.

## Config

`scheduler [config.json]` reads the JSON file, if given and found, then the environment, which wins. A field set by
neither takes its default. Logs are JSON, on stdout.

| Key | Env | Default |
| --- | --- | --- |
| `log.level` | `LOG_LEVEL` | `info`; `debug` logs each time fired |
| `sqlite.path` | `SQLITE_PATH` | `scheduler.db` |
| `mqtt.broker` | `MQTT_BROKER` | `mqtt://localhost:1883`, or `tls://…` |
| `mqtt.client_id` | `MQTT_CLIENT_ID` | none: required |
| `mqtt.prefix` | `MQTT_PREFIX` | none: topics are then `schedules/<op>` |

## Layout

| Path | Holds |
| --- | --- |
| `cmd/scheduler` | The binary: config, wiring, and the MQTT API, one file per op. |
| `internal/schedule` | The domain: `Schedule`, the `Store` and `Service` interfaces, their errors and mocks. |
| `internal/schedule/service` | `service.S`, which rings an alarm per enabled schedule. |
| `internal/schedule/sqlite` | The `Store`, on SQLite. |
| `pkg/` | Libraries with no knowledge of schedules: `config`, `log`, `mqtt`, `shutdown`, `sqlite`. |

## Development

```sh
make check     # lint, and tests with -race
make build     # bin/scheduler, without cgo: GOOS=linux GOARCH=arm64 make build cross-compiles
make run       # with config/scheduler/local.json, on the broker at localhost:1883
make generate  # the moq mocks of internal/, after an interface changed
```

## License

MIT.

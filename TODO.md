# TODO

## Short-term

- [ ] Real operations in place of `noop`, reading `Options.Params`.
- [ ] A bare RRULE, without DTSTART, starting from now: store the set with the DTSTART added, so that it reads back
  the same.

## Mid-term

- [ ] multicast between alarms and operations: `fn` only publishes a firing, non-blocking, to a `multicast.Group`, each
  operation being a subscriber filtered on its key, with its own buffer, timeout and retry, so that an alarm stays on
  time whatever its operation costs. One more subscriber publishes each firing to `<prefix>/schedules/<id>/fired`.
- [ ] Each schedule's state retained on `<prefix>/schedules/<id>`, so that a client subscribing gets them all.
- [ ] Broker credentials and TLS from `mqtt.Config` (`autopaho.ClientConfig.ConnectUsername`, `TlsCfg`).

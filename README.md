# TimeLord kid control

TimeLord monitors per-user processes and exposes them as Prometheus metrics. It
replaces an earlier .NET app.

- [Linux](docs/Linux.md)
- [Windows](docs/Windows.md)
- [Data model](docs/Model.md)
- [Timetower API](docs/Timetower.md)
- [Timetower OpenAPI spec](docs/openapi.yaml)

## Timetower

Timetower is the central REST service for Enforcers. It identifies Enforcers by
the client IP forwarded by the ingress, using static configuration.

```sh
cp deploy/timetower/config-default.json timetower.json
# Set each Enforcer's source IP in timetower.json.
go build -o timetower ./cmd/timetower
./timetower -config timetower.json -state timetower-state.json
```

Timetower serves plain HTTP; TLS terminates at the ingress. Deploy it with
`./deploy-timetower.sh`. The API is documented in
[docs/Timetower.md](docs/Timetower.md) and [docs/openapi.yaml](docs/openapi.yaml).

## Build and run

The Enforcer daemon is built from `cmd/timelord`:

```sh
go build -o timelord ./cmd/timelord
sudo ./timelord -user dani # root/LocalSystem can see other users
curl http://127.0.0.1:9220/metrics
```

TimeLord needs root on Linux or LocalSystem on Windows to read other users'
processes. It runs a power-on self-test at startup and refuses to start when a
check fails; run it alone with `-selftest` (exits non-zero on failure).

## Metrics

The service exposes metrics on `0.0.0.0:9220` at `/metrics` (`-listen` changes
the address). See [docs/Model.md](docs/Model.md).

## Schedule

TimeLord reads `~/timelord-schedule.json` from each user selected with the
repeatable `-user` flag. A missing file means that user is allowed no time. The
file is watched and reloaded automatically. On Linux, denied local sessions
are logged out via `systemd-logind`; remote sessions such as SSH are ignored.

See [docs/Model.md](docs/Model.md#schedule-data-model) and the sample
[timelord-schedule.dani.json](timelord-schedule.dani.json).

Daily usage is persisted in `~/timelord-schedule.csv` and restored on startup.
The deployment script currently passes `-user dani`. The remaining daily time
is exported as `timelord_schedule_remaining_seconds{user="dani"}`.

Upload Dani's schedule to the default `dani@dani` host with:

```sh
./deploy-schedule.sh
# or: ./deploy-schedule.sh user@host path/to/timelord-schedule.json
```

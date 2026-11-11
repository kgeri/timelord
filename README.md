# TimeLord kid control

TimeLord monitors per-user processes and exposes them as Prometheus metrics. It
replaces an earlier .NET app.

- [Linux](docs/Linux.md)
- [Windows](docs/Windows.md)
- [Data model](docs/Model.md)
- [Timetower API](docs/Timetower.md)
- [Timetower OpenAPI spec](docs/openapi.yaml)

## Timetower

Timetower is the central REST service for Enforcers. It identifies users by
account name. Enforcers authenticate with the shared `TIMELORD_KEY`.

```sh
cp deploy/timetower/config-default.json timetower.json
# Edit the users and policies in timetower.json.
go build -o timetower ./cmd/timetower
TIMELORD_KEY=... ./timetower -config timetower.json -state timetower-state.json
```

Timetower serves plain HTTP; TLS terminates at the ingress. Deploy it with
`./deploy-timetower.sh` (targets `srvu` by default). The API is documented in
[docs/Timetower.md](docs/Timetower.md) and [docs/openapi.yaml](docs/openapi.yaml).

## Build and run

The Enforcer daemon is built from `cmd/timelord`:

```sh
go build -o timelord ./cmd/timelord
sudo env TIMELORD_KEY=... ./timelord -timetower https://timetower.ogreg.org -user dani
curl http://127.0.0.1:9220/metrics
```

TimeLord needs root on Linux or LocalSystem on Windows to read other users'
processes. It runs a power-on self-test at startup and refuses to start when a
check fails; run it alone with `-selftest` (exits non-zero on failure).

## Metrics

The service exposes metrics on `0.0.0.0:9220` at `/metrics` (`-listen` changes
the address). See [docs/Model.md](docs/Model.md).

## Schedule

The Enforcer fetches its schedule from Timetower on startup and reports status
at the interval Timetower provides. Timetower is the source of truth; the
Enforcer reads no schedule or usage files from user home directories.

On Linux, denied local sessions are logged out via `systemd-logind`; remote
sessions such as SSH are ignored. The public status listing is available from:

```sh
curl https://timetower.ogreg.org/api/v1/enforcers
```

The shared key and deployment configuration are described in
[docs/Timetower.md](docs/Timetower.md).

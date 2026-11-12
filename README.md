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
account name. All clients use HTTP Basic auth: Enforcers use the `enforcer`
username with `TIMELORD_ENFORCER_KEY`, and the admin console uses the `admin`
username with `TIMELORD_ADMIN_PASSWORD`.

```sh
cp deploy/timetower/config-default.json timetower.json
# Edit the users and policies in timetower.json.
go build -o timetower ./cmd/timetower
TIMELORD_ENFORCER_KEY=... TIMELORD_ADMIN_PASSWORD=... \
  ./timetower -config timetower.json -state timetower-usage.csv
```

Timetower serves plain HTTP; TLS terminates at the ingress. Deploy it with
`./deploy-timetower.sh` (targets `srvu` by default). The API is documented in
[docs/Timetower.md](docs/Timetower.md) and [docs/openapi.yaml](docs/openapi.yaml).

The admin UI in [gui](gui) is built to `gui/build` and served by the same
binary. The UI and its assets are protected by the admin Basic auth:

```sh
(cd gui && npm ci && npm run build)
TIMELORD_ENFORCER_KEY=... TIMELORD_ADMIN_PASSWORD=... \
  ./timetower -config timetower.json -state timetower-usage.csv -web gui/build
# open https://.../ and sign in as admin
```

The Docker image builds the UI automatically.

## Build and run

The Enforcer daemon is built from `cmd/timelord`:

```sh
go build -o timelord ./cmd/timelord
sudo env TIMELORD_ENFORCER_KEY=... ./timelord -timetower https://timetower.ogreg.org -user dani
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
sessions such as SSH are ignored. The admin listing is available with the admin
Basic credentials:

```sh
curl -u "admin:$TIMELORD_ADMIN_PASSWORD" https://timetower.ogreg.org/api/v1/enforcers
```

The Enforcer key, admin password, and deployment configuration are described in
[docs/Timetower.md](docs/Timetower.md).

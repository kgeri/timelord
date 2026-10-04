# TimeLord kid control

TimeLord monitors per-user processes and exposes them as Prometheus metrics. It
replaces an earlier .NET app.

- [Linux](docs/Linux.md)
- [Windows](docs/Windows.md)
- [Data model](docs/Model.md)

## Build and run

```sh
go build -o timelord ./cmd/timelord
sudo ./timelord            # root/LocalSystem can see other users
curl http://127.0.0.1:9220/metrics
```

TimeLord needs root on Linux or LocalSystem on Windows to read other users'
processes. It runs a power-on self-test at startup and refuses to start when a
check fails; run it alone with `-selftest` (exits non-zero on failure).

## Metrics

The service exposes metrics on `0.0.0.0:9220` at `/metrics` (`-listen` changes
the address). See [docs/Model.md](docs/Model.md).

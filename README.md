# TimeLord kid control

This app was borne out of frustration with Microsoft Family Safety. The darned thing wasn't recording
usage stats nor applying limits for one of my kids so here we are.

Up until `5f7ae1dea2` this was a .NET app, but I decided to make this cross-platform and move to Go.

## Build and run

```sh
go build -o timelord ./cmd/timelord
./timelord
```

Or build and run in one step:

```sh
go run ./cmd/timelord
```

## Metrics

The service exposes Prometheus metrics on `0.0.0.0:9220` at `/metrics`.
Use `-listen` to change the address:

```sh
./timelord -listen 127.0.0.1:9220
curl http://127.0.0.1:9220/metrics
```

The endpoint exposes these metrics per `user` and `name`:

- `timelord_process_instances` - number of running processes
- `timelord_process_memory_rss_bytes` - resident memory
- `timelord_process_cpu_seconds_total` - CPU time that TimeLord observed, in seconds

TimeLord must run as root to read the executable path of other users' processes.
It logs a warning when it does not run as root.

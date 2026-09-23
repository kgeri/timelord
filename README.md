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

The metric `timelord_process` gives the number of processes per `user` and `name`.

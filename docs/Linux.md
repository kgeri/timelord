# Linux

## Build and run

```sh
go build -o timelord ./cmd/timelord
sudo ./timelord
```

TimeLord must run as root to read the executable path and memory of other
users' processes. The source is
[internal/process/lister_linux.go](../internal/process/lister_linux.go).

## Deploy

[deploy.sh](../deploy.sh) builds the binary, installs it and the systemd unit,
and restarts the service:

```sh
sudo ./deploy.sh [user@]host
sudo ./deploy.sh --uninstall [user@]host
```

- Unit: [deploy/timelord.service](../deploy/timelord.service)
- Scope: [internal/process/lister_linux.go](../internal/process/lister_linux.go)
  (only `app.slice` is `app`; session, background and system services are
  `system`)

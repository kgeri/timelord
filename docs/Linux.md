# Linux

## Build and run

```sh
go build -o timelord ./cmd/timelord
sudo ./timelord
```

TimeLord must run as root to read the executable path and memory of other
users' processes. The source is
[internal/process/lister_linux.go](../internal/process/lister_linux.go).

## Schedule enforcement

Pass one `-user` flag for each managed user. TimeLord reads
`~/timelord-schedule.json` from that user's home and watches it for changes.
Sessions come from systemd-logind: active, unlocked local sessions count toward
the daily limit, and a denied session is logged out with
`loginctl terminate-session`. Remote sessions such as SSH are ignored. Login,
logout, lock, and unlock transitions for managed users are logged with an
`AUDIT:` prefix. The source is [internal/enforce](../internal/enforce).

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

# Linux

## Build and run

```sh
go build -o timelord ./cmd/timelord
sudo env TIMELORD_ENFORCER_KEY=... ./timelord -timetower https://timetower.ogreg.org -user dani=dani
```

TimeLord must run as root to read the executable path and memory of other
users' processes. The source is
[internal/process/lister_linux.go](../internal/process/lister_linux.go).

## Schedule enforcement

The Enforcer fetches each configured user's schedule from Timetower on startup
and reports status at the interval Timetower provides. It reads no schedule or
usage files. Users are selected with the repeatable `-user` flag. Use
`local=server` to map a Linux account to a different Timetower account.

Sessions come from systemd-logind: active, unlocked local sessions count toward
the daily limit, and a denied session is logged out with
`loginctl terminate-session`. Remote sessions such as SSH are ignored. Login,
logout, lock, and unlock transitions for managed users are logged with an
`AUDIT:` prefix. The sources are [internal/enforcer](../internal/enforcer) and
[internal/session](../internal/session).

## Notifications

The Enforcer sends a short, silent notification when a child's daily limit or
time window is about to end. Because the service runs as root, the Linux
notifier re-executes the binary as the session user and calls the freedesktop
notification service on that user's session bus. See
[internal/notify](../internal/notify) and the threshold handling in
[internal/enforcer/agent.go](../internal/enforcer/agent.go).

Send a test notice to every active local session with:

```sh
sudo /usr/local/bin/timelord -notify-test
```

## Deploy

[deploy-enforcer-linux.sh](../deploy-enforcer-linux.sh) builds the binary,
installs it and the systemd unit, and restarts the service:

```sh
sudo ./deploy-enforcer-linux.sh [user@]host
sudo ./deploy-enforcer-linux.sh --uninstall [user@]host
```

The deploy script reads `secrets.env` for `TIMELORD_ENFORCER_KEY`, installs
`/etc/timelord.env` with mode `0600`, and the systemd unit loads it with
`EnvironmentFile`. Set `TIMETOWER_URL` to override the default
`https://timetower.ogreg.org`; set `TIMELORD_USERS` to override the default
`dani=dani` mapping.

- Unit: [deploy/timelord.service](../deploy/timelord.service)
- Scope: [internal/process/lister_linux.go](../internal/process/lister_linux.go)
  (only `app.slice` is `app`; session, background and system services are
  `system`)

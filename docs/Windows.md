# Windows

## Build and run

```sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o timelord.exe ./cmd/timelord
```

Run `timelord.exe -timetower https://timetower.ogreg.org` from a console, or
install it as a LocalSystem service. On the host, from an elevated session:

```powershell
.\install.ps1
```

## Deploy over SSH

The host needs an OpenSSH server and an elevated session. Use the
built-in Administrator, or set `LocalAccountTokenFilterPolicy` to 1 under
`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`.
[deploy-enforcer-windows.sh](../deploy-enforcer-windows.sh) does both steps:

```sh
./deploy-enforcer-windows.sh [user@]host
./deploy-enforcer-windows.sh --uninstall [user@]host
```

Set `TIMETOWER_URL` to override the default
`https://timetower.ogreg.org`.

Logs at `%ProgramData%\TimeLord\timelord.log`. Scope is `system` for session 0,
system/service accounts, and known Windows component paths/packages. See
[install.ps1](../deploy/windows/install.ps1), [lister_windows.go](../internal/process/lister_windows.go).

Schedule enforcement is currently Linux-only; Windows still exposes the
process metrics. The installer registers the service with the Timetower URL,
but the Timetower agent is not started without a session source.

TODO: Windows Enforcers will need a protected key file or service
environment before the Timetower client is enabled.

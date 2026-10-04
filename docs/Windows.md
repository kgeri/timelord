# Windows

## Build and run

```sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o timelord.exe ./cmd/timelord
```

Run `timelord.exe` from a console, or install it as a LocalSystem
service. On the host, from an elevated session:

```powershell
.\install.ps1
```

## Deploy over SSH

The host needs an OpenSSH server and an elevated session. Use the
built-in Administrator, or set `LocalAccountTokenFilterPolicy` to 1 under
`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`.
[deploy-windows.sh](../deploy-windows.sh) does both steps:

```sh
./deploy-windows.sh [user@]host
./deploy-windows.sh --uninstall [user@]host
```

Logs at `%ProgramData%\TimeLord\timelord.log`. Scope is `system` for session 0,
system/service accounts, and known Windows component paths/packages. See
[install.ps1](../deploy/windows/install.ps1), [lister_windows.go](../internal/process/lister_windows.go).

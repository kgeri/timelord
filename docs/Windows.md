# Windows

## Build and run

```sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o timelord.exe ./cmd/timelord
```

Run `timelord.exe -timetower https://timetower.ogreg.org -user kissd=dani -user kissr=reka` from a
console with `TIMELORD_ENFORCER_KEY` set, or pass `-key-file` with a protected
key file. To install it as a LocalSystem service, run the following on the host
from an elevated session:

```powershell
.\install.ps1 -Users 'kissd=dani kissr=reka' -EnforcerKeyFile .\timelord.key
```

The installer copies the key to `%ProgramData%\TimeLord\timetower.key` and
restricts its ACL to LocalSystem and the Administrators group. It registers the
service with the configured users.

## Deploy over SSH

The host needs an OpenSSH server and an elevated session. Use the
built-in Administrator, or set `LocalAccountTokenFilterPolicy` to 1 under
`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`.
[deploy-enforcer-windows.sh](../deploy-enforcer-windows.sh) does both steps:

```sh
./deploy-enforcer-windows.sh [user@]host
./deploy-enforcer-windows.sh --uninstall [user@]host
```

The deploy script reads `secrets.env` for `TIMELORD_ENFORCER_KEY`, like the
Linux script. Set `TIMETOWER_URL` to override the default
`https://timetower.ogreg.org`, and set `TIMELORD_USERS` to override the default
`kissd=dani kissr=reka` mappings.

Logs are written to `%ProgramData%\TimeLord\timelord.log`. Scope is `system` for
session 0, system/service accounts, and known Windows component
paths/packages. See [install.ps1](../deploy/windows/install.ps1) and
[lister_windows.go](../internal/process/lister_windows.go).

## Schedule enforcement

The Enforcer fetches each configured user's schedule from Timetower and enforces
it locally, like the Linux Enforcer. Windows sessions come from the Terminal
Services API (`wtsapi32.dll`) in
[session_windows.go](../internal/session/session_windows.go):

- sessions are discovered with `WTSEnumerateSessions` and
  `WTSQuerySessionInformation`;
- every logged-on user session is treated as local; RDP/console distinction is
  not tracked;
- only active, unlocked sessions count toward the daily limit;
- session 0 (services) and sessions without a logged-on user are skipped;
- when a counted active session is denied, it is logged off with
  `WTSLogoffSession`. This closes the user's programs, matching the Linux
  `loginctl terminate-session` behavior;
- login, logout, lock, and unlock transitions are logged with the `AUDIT:`
  prefix.

The Enforcer sends a short, silent notification when a child's daily limit or
time window is about to end. Because the service runs as LocalSystem, it
launches a short-lived helper in the child's interactive session and shows a
tray balloon, which Windows 11 renders as a toast. See
[internal/notify](../internal/notify) and the threshold handling in
[internal/enforcer/agent.go](../internal/enforcer/agent.go).

Send a test notice to every active local session by running the binary as
LocalSystem. An elevated administrator session is not enough, because
`WTSQueryUserToken` requires `SeTcbPrivilege`.

Account names are compared case-insensitively on Windows. The local part of the
`-user` mapping is the Windows account name; the server part is the Timetower
account. For example, `-user kissd=dani` counts Windows `kissd` activity against
Timetower user `dani`, and `-user kissr=reka` maps `kissr` to `reka`. Local and
Microsoft account logins use their Windows account name; domain accounts are not
supported. The Windows Enforcer does not
read any schedule or usage files. The binary embeds the IANA time zone database,
so Timetower time zone names such as `Europe/Budapest` work without installing a
separate zoneinfo database.

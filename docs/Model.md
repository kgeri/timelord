# Data model

The Prometheus data model lives in
[internal/metrics/metrics.go](../internal/metrics/metrics.go). TimeLord collects
one [process.Process](../internal/process/process.go) per running process and
aggregates it per `user`, `name`, and `scope`.

Timetower owns the schedule, policy, and usage model. It is documented in
[Timetower.md](Timetower.md).

## Prometheus metrics

| Metric | Type | Labels |
|--------|------|--------|
| `timelord_process_instances` | gauge | `user`, `name`, `scope` |
| `timelord_process_memory_rss_bytes` | gauge | `user`, `name`, `scope` |
| `timelord_process_cpu_seconds_total` | counter | `user`, `name`, `scope` |

## Process fields

| Field | Meaning | Linux | Windows |
|-------|---------|-------|---------|
| `PID` | Process ID | `/proc/<pid>` directory name | `ProcessEntry32.ProcessID` |
| `User` | Owner name | `stat()` UID via `user.LookupId` | token `TokenUser` SID via `LookupAccount` |
| `Name` | Short name | `/proc/<pid>/comm`, else executable base | image base, else toolhelp `ExeFile` |
| `Executable` | Executable path | `/proc/<pid>/exe` symlink | `QueryFullProcessImageName` |
| `Scope` | `app` or `system` | `app.slice` in `/proc/<pid>/cgroup` | session 0, system account, or a system component path/package |
| `MemoryBytes` | Resident memory | `/proc/<pid>/stat` rss × page size | `GetProcessMemoryInfo` working set |
| `CPUSeconds` | CPU time | `/proc/<pid>/stat` utime+stime ÷ 100 | `GetProcessTimes` kernel+user |
| `StartTime` | Instance identity | `/proc/<pid>/stat` starttime | `GetProcessTimes` creation FILETIME |

The `Scope` values are defined in
[internal/process/process.go](../internal/process/process.go), and the mappings
live in [lister_linux.go](../internal/process/lister_linux.go) and
[lister_windows.go](../internal/process/lister_windows.go).

## Schedule data model

Timetower stores users, policies, reporting hostnames, and usage. The Enforcer
fetches schedules from Timetower and reports usage back; it does not read or
write schedule or usage files. See [Timetower.md](Timetower.md) for the
authoritative model and API.

The enforcement rules remain:

- a **window** is a time of day when a user may use the computer;
- a **daily limit** is the maximum active seconds per policy day;
- a user with no policy, a disabled policy, or no window for today is denied;
- usage is aggregated across all hostnames for the user;
- a daily limit of `0` means no duration limit.

On Linux, sessions come from systemd-logind. A local session is active when it
is on a seat, `Remote=no`, `Active=yes`, and `LockedHint=no`. Denied local
sessions are logged out with `loginctl terminate-session`; remote sessions such
as SSH are ignored. Login, logout, lock, and unlock changes for managed users are
written to the log with an `AUDIT:` prefix. The agent loop lives in
[internal/enforcer](../internal/enforcer); the platform session source lives in
[internal/session](../internal/session).

The remaining daily time for every reporting hostname is available from the
public `GET /api/v1/enforcers` endpoint; it is no longer exported as a
Prometheus metric.

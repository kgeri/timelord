# Data model

The Prometheus data model lives in
[internal/metrics/metrics.go](../internal/metrics/metrics.go). TimeLord collects
one [process.Process](../internal/process/process.go) per running process and
aggregates it per `user`, `name`, and `scope`.

The scheduler uses a separate model: a per-user policy that says when a user
may use the computer, plus the usage that TimeLord records for the current day.

## Prometheus metrics

| Metric | Type | Labels |
|--------|------|--------|
| `timelord_process_instances` | gauge | `user`, `name`, `scope` |
| `timelord_process_memory_rss_bytes` | gauge | `user`, `name`, `scope` |
| `timelord_process_cpu_seconds_total` | counter | `user`, `name`, `scope` |
| `timelord_schedule_remaining_seconds` | gauge | `user` |

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

A **policy** has two independent rules, and both must pass for the user to use
the computer:

- a **window** is a time of day when the user may use the computer;
- a **daily limit** is the maximum active time the user may use in one day.

Only users with a policy can be allowed. A user without a policy is denied, so
a missing schedule file denies that user.

The schedule is read from `~/timelord-schedule.json` in each managed user's
home directory. The service selects users with the repeatable `-user` flag. The
file is loaded at startup and watched for changes; an invalid reload is logged
and the previous schedule is kept, while a missing file means no time is
allowed. The Go model and file provider live in
[internal/schedule](../internal/schedule); `schedule.Provider` is the extension
point for a future REST source.

### Configuration

The root object has a `users` array. The file in a user's home contributes that
user's policy; other entries are ignored. User names must be unique. The sample
[timelord-schedule.dani.json](../timelord-schedule.dani.json) allows Dani to
use the computer every day between 08:00 and 19:00 for at most 60 active
minutes per day.

| `UserPolicy` field | Type | Meaning |
|--------------------|------|---------|
| `user` | string | Account name, as reported by the process lister. |
| `daily_limit_minutes` | integer | Maximum active minutes per local day. Omitted or `0` means no duration limit. |
| `windows` | array | Permitted intervals. A day with no window is not permitted. |

| `Window` field | Type | Meaning |
|----------------|------|---------|
| `day` | string | `mon`, `tue`, `wed`, `thu`, `fri`, `sat`, or `sun`. |
| `start` | `HH:MM` | Start of the interval, inclusive. |
| `end` | `HH:MM` | End of the interval, exclusive. Must be later than `start`; a window cannot cross midnight. |

Times are wall-clock times in the host's local time zone. There is no
per-user time zone.

Example: Dani may use the computer between 08:00 and 19:00 every day, for at
most 60 active minutes per day.

```json
{
  "users": [
    {
      "user": "dani",
      "daily_limit_minutes": 60,
      "windows": [
        {"day": "mon", "start": "08:00", "end": "19:00"},
        {"day": "tue", "start": "08:00", "end": "19:00"},
        {"day": "wed", "start": "08:00", "end": "19:00"},
        {"day": "thu", "start": "08:00", "end": "19:00"},
        {"day": "fri", "start": "08:00", "end": "19:00"},
        {"day": "sat", "start": "08:00", "end": "19:00"},
        {"day": "sun", "start": "08:00", "end": "19:00"}
      ]
    }
  ]
}
```

Several windows on the same day are allowed and share the same daily limit.
Overlapping windows behave as their union.

### Usage state

TimeLord keeps one usage record for each managed user and local date:

| `Usage` field | Type | Meaning |
|---------------|------|---------|
| `user` | string | Account name. |
| `date` | `YYYY-MM-DD` | Local calendar day. |
| `used_seconds` | integer | Active seconds counted for the user on that date. |

A new record starts at zero at local midnight. Records for earlier dates are
discarded. Used time increases only while the user has an active session
(logged on and unlocked); locked or logged-off time does not count. The daily
limit is shared by all windows on that date. The current implementation keeps
usage in memory, so restarting TimeLord resets the current day's used time.

### Evaluation

At time `now`, a managed user is allowed to use the computer when all of these
are true:

1. `now` is inside one of the user's windows for the current local weekday.
2. `daily_limit_minutes` is `0`, or `used_seconds` is less than
   `daily_limit_minutes * 60`.

Otherwise the user is denied. A managed user with no window for today is
denied for the whole day.

On Linux, TimeLord reads sessions from systemd-logind. A local session is active
when it is on a seat, `Remote=no`, `Active=yes`, and `LockedHint=no`. Denied
local sessions are logged out with `loginctl terminate-session`; remote sessions
such as SSH are ignored. Login, logout, lock, and unlock changes for managed
users are written to the log with an `AUDIT:` prefix. The session
implementation lives in [session_linux.go](../internal/enforce/session_linux.go);
the usage tracker and termination loop live in
[enforce.go](../internal/enforce/enforce.go).

`timelord_schedule_remaining_seconds` reports the remaining active seconds for
users with a positive daily limit. A managed user with no schedule reports
zero. Users without a daily limit have no series.

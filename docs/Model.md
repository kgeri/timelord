# Data model

The Prometheus data model lives in
[internal/metrics/metrics.go](../internal/metrics/metrics.go). TimeLord collects
one [process.Process](../internal/process/process.go) per running process and
aggregates it per `user`, `name`, and `scope`.

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

# Timetower

Timetower is the central REST service for TimeLord. Enforcers are per-host
daemons that measure active time and enforce the schedule that Timetower
provides.

Timetower identifies an Enforcer by the source IP address of its request. The
address-to-Enforcer mapping is static configuration. The service is intended for
LAN use only.

Schedules, usage, and status are stored by Timetower. Enforcers do not read
schedule or usage files from user home directories.

The API contract is in [openapi.yaml](openapi.yaml).

## Components

| Component | Responsibility |
|-----------|----------------|
| Timetower | Central REST service. Stores subjects, policies, usage, and the latest Enforcer status. |
| Enforcer | Per-host daemon. Reads sessions and processes, applies policy locally, terminates denied local sessions, reports status. |
| Operator | Maintains Timetower's static configuration. |

## Static configuration

Timetower reads a JSON configuration file at startup and fails when the file is
missing. The default is `deploy/timetower/config-default.json`; the deployment
keeps the live file at `/data/hosted/timetower/config.json`.

The configuration maps each Enforcer's source IP address to its managed
subjects and OS accounts. An Enforcer receives only the subjects configured for
its address. Requests from unconfigured addresses are rejected.

Timetower uses the rightmost `X-Forwarded-For` address set by the ingress,
falling back to `X-Real-IP` and the direct peer address.

## Model

### Enforcer

An Enforcer represents one managed host. It is identified by its source IP
address. Its configured name, subjects, and OS accounts are static
configuration. Host details such as version, health, sessions, and process
statistics are reported in the status request.

### Subject and policy

A subject is a managed identity, for example `dani`. A subject has an optional
policy:

- `enabled` turns the policy on or off.
- `daily_limit_seconds` is the maximum active time per policy day. `0` means no
  duration limit.
- `windows` lists permitted weekly intervals.

A window has a weekday, a start time, and an end time. Start is inclusive, end
is exclusive, and a window cannot cross midnight. A day with no window is
denied. Several windows on the same day are allowed and overlap as their union.

### Usage

Timetower stores active seconds per Enforcer, subject, and policy day. Each
Enforcer reports an absolute, monotonic counter in its status request.
Timetower stores the maximum counter reported for that Enforcer and day, and
sums the per-Enforcer counters to obtain subject usage.

The daily limit is shared across all Enforcers. Active time on two hosts at the
same time consumes the limit twice as fast.

Counters reset at midnight in Timetower's policy time zone. Before resetting,
an Enforcer moves the old day's counter into a pending set so unreported usage
is not lost.

### Status

An Enforcer sends its current health, per-subject session state, usage counters,
and process statistics in every status request. Timetower stores the latest
status per Enforcer.

Both schedule and status responses carry an `X-Schedule-Revision` header.
When the status response returns a different revision than the one the
Enforcer cached from the schedule, the Enforcer fetches the schedule again.

## Enforcer lifecycle

1. The Enforcer loads its configuration and offline cache.
2. It fetches the schedule from `GET /api/v1/schedule`.
3. It starts enforcement and status timers.
4. It sends `PUT /api/v1/status` every `status_interval_seconds`.
5. It fetches the schedule again when `X-Schedule-Revision` changes.
6. It sends a final status request during clean shutdown.

The schedule response contains the policy, policy time zone,
`status_interval_seconds`, `offline_grace_seconds`, and the subject's usage and
remaining time. The status response contains authoritative aggregate usage,
remaining time, and the current policy day.

## Enforcement

An assigned subject is allowed when all of these are true:

1. The subject has a policy.
2. The policy is enabled.
3. The current wall-clock time in Timetower's policy time zone is inside one of
   the subject's windows for the current weekday.
4. There is no daily limit, or estimated usage is below the limit.

Otherwise the subject is denied.

The Enforcer evaluates this locally on every enforcement tick. The `allowed`
field in the status response is advisory; a missed status request does not grant
time.

A session counts toward usage when it is local, active, and unlocked. Remote
sessions such as SSH do not count and are not terminated. A denied local session
is terminated with the platform mechanism. Login, logout, lock, unlock, and
termination events are logged with the `AUDIT:` prefix.

## Offline operation

The Enforcer keeps a root-owned cache containing the last schedule, policy time
zone, intervals, usage counters, aggregate usage, and clock offset.

When Timetower is unreachable, the Enforcer continues with the cached schedule
and usage while the cache is younger than `offline_grace_seconds`. After that
grace period it denies all configured subjects until a successful sync. Pending
usage is reported on the next successful status request.

If no cache exists and Timetower is unreachable, the Enforcer has no subjects,
logs a critical error, and retries with backoff.

The cache is not a schedule source and is not writable by managed users.

## Time

Timetower has one configured IANA policy time zone. Policy windows, daily
limits, and day rollover use that time zone, not the host's local time zone.

All API timestamps are UTC RFC 3339. Durations are integer seconds. Elapsed
active time is measured with a monotonic clock and capped per tick to avoid
charging suspend or stall time.

## API

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/schedule` | Fetch the Enforcer's policy, usage, and remaining time. |
| `PUT` | `/api/v1/status` | Report health, usage, session state, and process statistics. |

Both success responses carry `X-Schedule-Revision`. The Enforcer caches the
value from the schedule and fetches the schedule again when a status response
returns a different value.

Timetower serves plain HTTP. The k3s ingress terminates TLS for
`timetower.ogreg.org`.

## Storage and retention

Timetower reads its static configuration and stores daily usage and the latest
status per Enforcer in its state file.

Daily usage is retained for 400 days. The latest status is retained per
Enforcer.

## Deployment

`deploy-timetower.sh` seeds `/data/hosted/timetower/config.json` from
`deploy/timetower/config-default.json` when it is missing, builds and pushes the
Timetower image, and applies the Kubernetes manifests in
`deploy/timetower/deployment.yaml`. Pass a `[user@]host` argument to seed the
config over SSH/SCP; without one, the script assumes it runs on the host that
owns `/data/hosted/timetower`.

Usage and status state is stored alongside the configuration in
`/data/hosted/timetower/state.json`.

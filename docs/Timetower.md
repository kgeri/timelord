# Timetower

Timetower is the central REST service for TimeLord. Enforcers are per-host
daemons that measure active time and enforce the schedule that Timetower
provides.

Timetower identifies users by account name. Enforcers authenticate with a shared
bearer key and report their own hostname for usage aggregation and listing.
Source IP addresses and ingress forwarding headers are not used.

Schedules, usage, and status are stored by Timetower. Enforcers do not use
schedule or usage files.

The API contract is in [openapi.yaml](openapi.yaml).

## Components

| Component | Responsibility |
|-----------|----------------|
| Timetower | Central REST service. Stores users, policies, usage, and the latest status per hostname. |
| Enforcer | Per-host daemon. Fetches user schedules, reads sessions and processes, applies policy locally, terminates denied local sessions, reports status. |
| Operator | Maintains Timetower's static configuration and the shared key. |

## Static configuration

Timetower reads a JSON configuration file at startup and fails when the file is
missing or invalid. The default is `deploy/timetower/config-default.json`; the
deployment keeps the live file at `/data/hosted/timetower/config.json`.

The configuration contains users and their policies:

```json
{
  "timezone": "Europe/Budapest",
  "status_interval_seconds": 60,
  "offline_grace_seconds": 86400,
  "users": [
    {
      "name": "dani",
      "display_name": "Dani",
      "policy": {
        "enabled": true,
        "windows": [
          {"day": "mon", "start": "08:00", "end": "19:00", "limit_minutes": 60},
          {"day": "tue", "start": "08:00", "end": "20:30", "limit_minutes": 60}
        ]
      }
    }
  ]
}
```

A user with an invalid or missing policy is denied. There is no separate
Enforcer-to-account mapping.

## Authentication

The Timetower API uses one shared bearer key:

```http
Authorization: Bearer <TIMELORD_KEY>
```

The schedule, status, and override endpoints require the key. The
`GET /api/v1/enforcers` listing is public for now; access to an admin UI will
be guarded separately later.

The key is distributed through `secrets.env`:

```sh
TIMELORD_KEY=...
```

`secrets.env` must be mode `0600` or `0400` and is gitignored.

The key is shared across all Enforcers. Any holder can fetch or report any user.
Per-user keys would be required to isolate Enforcers from each other.

## Model

### User and policy

A user is an OS account name, for example `dani`. A user has an optional policy:

- `enabled` turns the policy on or off.
- `windows` lists permitted weekly intervals, each with the daily limit for its
  weekday in minutes.

A window has a weekday, a start time, an end time, and `limit_minutes`. Start is
inclusive, end is exclusive, and a window cannot cross midnight. A day with no
window is denied. Several windows on the same day are allowed and overlap as
their union; their `limit_minutes` values must match because the limit applies
to the whole policy day. A missing or `0` limit means no duration limit. An
existing policy-level `daily_limit_seconds` must be moved into each window's
`limit_minutes`; the field is not accepted in the configuration anymore.

The schedule API keeps its existing `daily_limit_seconds` field. Timetower
converts the configured minutes for the current policy day to seconds, and the
schedule revision changes with the day so Enforcers refresh the current limit.

### Current-day override

An operator can override the current day's window and/or daily limit without
editing the static configuration:

```sh
curl -X PUT https://timetower.ogreg.org/api/v1/override/dani \
  -H "Authorization: Bearer $TIMELORD_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"start": "10:00", "end": "18:00", "limit_minutes": 15}'
```

All three fields are optional. Omitted fields keep the configured value or an
existing override. When the day has a configured window, setting `start` or
`end` to the configured boundary removes that field's override, and setting
`limit_minutes` to the configured value removes the limit override. When
nothing remains, the whole override is cleared; there is no separate reset
call. `limit_minutes: 0` means no duration limit. A low limit applies
immediately, so a user already above it is locked out on the next status
check.

The configured start and end for the day are its earliest start and latest end.
Overriding either boundary replaces the day's windows with one interval using
the effective start and end. A limit-only override keeps the configured
windows. A day with no configured window defaults to 00:00-23:59, so setting
`start` or `end` creates a window and allows that day.

Overrides are persisted with usage in the state CSV and keyed by policy day, so
they stop applying when the day changes. Changing the effective policy changes
`X-Schedule-Revision`, so Enforcers fetch the updated schedule. A successful
response returns the effective `windows` for the day and `limit_minutes`.

### Usage

Timetower stores active seconds per hostname, user, and policy day. Each
Enforcer reports one absolute, monotonic `usage_seconds` counter in its status
request. Timetower stores the maximum counter reported for that hostname and the
policy day inferred from `observed_at`, and sums the per-hostname counters to
obtain user usage.

The daily limit for the current policy day is shared across all hostnames.
Active time on two hosts at the same time consumes the limit twice as fast.

Counters reset at midnight in Timetower's policy time zone. Only the current
policy day is reported; usage that cannot be delivered before the day changes is
discarded.

### Status

An Enforcer sends its hostname, per-user session state, usage counter, and
process statistics in every status request. Timetower stores the latest status
per hostname and user.

Both schedule and status responses carry an `X-Schedule-Revision` header.
When the status response returns a different revision than the one the
Enforcer remembered from the schedule, the Enforcer fetches schedules again.

Timetower logs when a hostname transitions online and when its heartbeat is
lost after three status intervals.

## Enforcer lifecycle

1. The Enforcer starts with its Timetower URL, shared key, hostname, and list of
   managed users.
2. It fetches each user's schedule from `GET /api/v1/schedule/{user}`. A failed
   initial fetch is a startup failure.
3. It starts enforcement and status timers.
4. It sends `PUT /api/v1/status/{user}` for each user every
   `status_interval_seconds`.
5. It fetches schedules again when `X-Schedule-Revision` changes.
6. It sends final status requests during clean shutdown.

## Enforcement

A user is allowed when all of these are true:

1. The user has a policy.
2. The policy is enabled.
3. The current wall-clock time in Timetower's policy time zone is inside one of
   the user's windows for the current weekday.
4. There is no daily limit, or estimated usage is below the limit.

Otherwise the user is denied.

The Enforcer evaluates this locally on every enforcement tick. The `allowed`
field in the status response is advisory; a missed status request does not grant
time.

A session counts toward usage when it is local, active, and unlocked. Remote
sessions such as SSH do not count and are not terminated. A denied local session
is terminated with the platform mechanism. Login, logout, lock, unlock, and
termination events are logged with the `AUDIT:` prefix.

## Offline operation

The Enforcer keeps the last schedules, policy time zone, usage counters, and
aggregate usage in memory. It reads and writes no schedule or usage files.

If Timetower is unreachable at startup, the Enforcer exits and the service
manager restarts it. If contact is lost later, the Enforcer keeps enforcing the
in-memory schedules and estimates usage locally. After `offline_grace_seconds`
without a successful schedule or status call, it denies all configured users
until a sync succeeds. Usage from before the day change is not retried.

## Time

Timetower has one configured IANA policy time zone. Policy windows, daily
limits, and day rollover use that time zone, not the host's local time zone.

All API timestamps are UTC RFC 3339. Durations are integer seconds. Elapsed
active time is measured with a monotonic clock and capped per tick to avoid
charging suspend or stall time.

## API

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| `GET` | `/api/v1/schedule/{user}` | bearer key | Fetch one user's policy, usage, and remaining time. |
| `PUT` | `/api/v1/status/{user}` | bearer key | Report usage, session state, and process statistics. |
| `PUT` | `/api/v1/override/{user}` | bearer key | Override the current day's window and/or limit. |
| `GET` | `/api/v1/enforcers` | public | List reporting hostnames, online state, last contact, and remaining time. |

Both schedule and status success responses carry `X-Schedule-Revision`.
Override responses also carry it. The Enforcer remembers the value from the
schedule and fetches schedules again when a status response returns a different
value.

Timetower serves plain HTTP. The k3s ingress terminates TLS for
`timetower.ogreg.org`.

## Storage and retention

Timetower reads its static configuration and stores usage and current-day
overrides in a CSV file. Status and process statistics stay in memory and are
repopulated by the next status reports. The Enforcer stores no schedule or
usage files.

The CSV columns are `date,user,hostname,used_seconds,start,end,limit_minutes`.
Usage rows fill `hostname` and `used_seconds`; override rows leave those empty
and fill the override columns. Files with the older four-column usage-only
header still load. Usage and overrides are retained for 400 days.

## Deployment

`deploy-timetower.sh` seeds `/data/hosted/timetower/config.json` on `srvu`
from `deploy/timetower/config-default.json` when it is missing, builds and
pushes the Timetower image, and applies the Kubernetes manifests in
`deploy/timetower/deployment.yaml`. Set `TIMETOWER_FORCE_CONFIG=1` to replace an
existing config with the default.

The script also creates/updates the `timetower-secrets` Kubernetes Secret from
`secrets.env`. The Deployment reads `TIMELORD_KEY` from that Secret as an
environment variable.

The Deployment is pinned to `srvu` so its hostPath volumes are always on the
same node. Pass a `[user@]host` argument or set `TIMETOWER_HOST` to override
the SSH target; set `TIMETOWER_NODE` to override the Kubernetes node name.

Usage is stored alongside the configuration in
`/data/hosted/timetower/usage.csv`. The deploy script reads the directory
owner's UID/GID and runs the container as that user, so the pod does not need to
run as root.

On Linux, `deploy-enforcer-linux.sh` installs `/etc/timelord.env` with mode
`0600`, and the systemd unit loads it with `EnvironmentFile`. Set
`TIMELORD_USERS` on the deploy command to override the default `dani` account.

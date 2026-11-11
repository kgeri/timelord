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
        "daily_limit_seconds": 3600,
        "windows": [
          {"day": "mon", "start": "08:00", "end": "19:00"}
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

The schedule and status endpoints require the key. The `GET /api/v1/enforcers`
listing is public for now; access to an admin UI will be guarded separately later.

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
- `daily_limit_seconds` is the maximum active time per policy day. `0` means no
  duration limit.
- `windows` lists permitted weekly intervals.

A window has a weekday, a start time, and an end time. Start is inclusive, end
is exclusive, and a window cannot cross midnight. A day with no window is
denied. Several windows on the same day are allowed and overlap as their union.

### Usage

Timetower stores active seconds per hostname, user, and policy day. Each
Enforcer reports an absolute, monotonic counter in its status request.
Timetower stores the maximum counter reported for that hostname and day, and
sums the per-hostname counters to obtain user usage.

The daily limit is shared across all hostnames. Active time on two hosts at the
same time consumes the limit twice as fast.

Counters reset at midnight in Timetower's policy time zone. Before resetting,
an Enforcer moves the old day's counter into a pending set so unreported usage
is not lost.

### Status

An Enforcer sends its hostname, health, per-user session state, usage counters,
and process statistics in every status request. Timetower stores the latest
status per hostname and user.

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
until a sync succeeds. Pending usage is reported on the next successful status
request.

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
| `PUT` | `/api/v1/status/{user}` | bearer key | Report health, usage, session state, and process statistics. |
| `GET` | `/api/v1/enforcers` | public | List reporting hostnames, online state, last contact, and remaining time. |

Both schedule and status success responses carry `X-Schedule-Revision`. The
Enforcer remembers the value from the schedule and fetches schedules again when
a status response returns a different value.

Timetower serves plain HTTP. The k3s ingress terminates TLS for
`timetower.ogreg.org`.

## Storage and retention

Timetower reads its static configuration and stores daily usage and the latest
status per hostname and user in its state file. The Enforcer stores no schedule
or usage files.

Daily usage is retained for 400 days. The latest status is retained per
hostname and user.

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

Usage and status state is stored alongside the configuration in
`/data/hosted/timetower/state.json`.

On Linux, `deploy-enforcer-linux.sh` installs `/etc/timelord.env` with mode
`0600`, and the systemd unit loads it with `EnvironmentFile`. Set
`TIMELORD_USERS` on the deploy command to override the default `dani` account.

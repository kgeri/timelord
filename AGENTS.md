# Agent notes

## Test data

- All test fixtures live under `/testdata` at the repository root.
- Do not put JSON request/response fixtures beside Go files or inline them in
  tests. Read them with the existing `testdataPath`/`readTestdata` helpers.
- Group fixtures by feature: configuration under `testdata/config`, status
  under `testdata/status`, overrides under `testdata/override`.
- Add or update fixtures together with the behavior they cover. Every new API
  behavior needs request and response fixtures when applicable.

## Units in wire formats

- Timetower's static configuration uses `limit_minutes`.
- The Enforcer-facing schedule and status API intentionally uses
  `daily_limit_seconds`; `testdata/schedule.json` and the status response
  fixtures are correct.
- The override API uses `limit_minutes`.

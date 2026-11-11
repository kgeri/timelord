# Agent notes

## Test data

- All test fixtures live under `/testdata` at the repository root.
- Non-BDD tests read JSON fixtures with the existing `testdataPath`/
  `readTestdata` helpers; do not inline JSON in Go tests.
- Group fixtures by feature: configuration under `testdata/config`, status
  under `testdata/status`, overrides under `testdata/override`.
- Keep JSON fixtures for malformed payloads, large bodies, and wire-format
  golden files. Small BDD inputs and expected values belong in Gherkin tables
  so the scenario stays readable on its own.

## Tests

- BDD specifications for observable Timetower HTTP behavior live in
  `/testdata/features` and run through godog from the package under test.
  Step definitions use the `*_bdd_test.go` suffix.
- Decode Gherkin tables with `internal/gherkin.DecodeTable` and `table:"..."`
  struct tags instead of hand-written key switches.
- Prefer domain vocabulary (`the admin sets the override`, `the enforcer
  reports`, `the status is`) over HTTP plumbing in scenarios.
- Keep internal helpers, parsing, and platform-specific logic as ordinary Go
  table tests; Gherkin adds indirection without improving those.

## Units in wire formats

- Timetower's static configuration uses `limit_minutes`.
- The Enforcer-facing schedule and status API intentionally uses
  `daily_limit_seconds`; `testdata/schedule.json` and the status response
  fixtures are correct.
- The override API uses `limit_minutes`.

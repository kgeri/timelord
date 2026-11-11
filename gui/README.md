# Timetower Admin UI

A mobile-first admin console for Timetower: schedules, remaining daily time,
emergency locks, quick time extensions, weekly usage, and enforcer nodes.

The visual design follows [`docs/skeleton.html`](docs/skeleton.html) and the
behaviour is described in [`docs/SPEC.md`](docs/SPEC.md).

## Running

```sh
npm install
npm run dev
```

The app starts with in-memory mock data, so no backend is required. The mock
service follows the same wire shapes and policy rules as the real API.

```sh
npm run check   # svelte-check + TypeScript
npm run build   # production build
npm run preview # preview the production build
```

## API types and services

The wire types are generated from the canonical OpenAPI document at
[`../docs/openapi.yaml`](../docs/openapi.yaml):

```sh
npm run generate:api
```

This writes `src/lib/api/schema.d.ts`. Generation runs automatically before
`dev`, `build`, and `check`, and the generated file is committed so a plain
`vite build` also works.

The service seam is `TimetowerService` in `src/lib/api/types.ts`:

| Implementation | File | Used when |
|---|---|---|
| `MockTimetowerService` | `src/lib/api/mock.ts` | default / `PUBLIC_TIMETOWER_MODE=mock` |
| `HttpTimetowerService` | `src/lib/api/http.ts` | `PUBLIC_TIMETOWER_MODE=http` |

`src/lib/api/index.ts` selects the implementation from public environment
variables. UI components only see the interface, so swapping data sources does
not touch the views.

### Connecting to a real Timetower

Copy `.env.example` to `.env` and set `PUBLIC_TIMETOWER_MODE=http`. The default
HTTP path uses the same-origin proxy at `src/routes/api/v1/[...path]/+server.ts`:

```sh
TIMETOWER_API_URL=https://timetower.ogreg.org
TIMETOWER_API_TOKEN=<shared TIMELORD_KEY>
```

The proxy is the recommended setup: the bearer key stays server-side, and the
browser avoids CORS. For direct development access, set
`PUBLIC_TIMETOWER_API_URL` to the Timetower origin; a public token is supported
but should not be used in production because it is shipped to the browser.

Set `PUBLIC_TIMETOWER_USERS` to the accounts that should appear in the carousel.
Timetower currently exposes no configured-user listing, so the HTTP service
unions this list with users found in `/api/v1/enforcers`.

### Known API gaps

- `/api/v1/enforcers` has no `X-Schedule-Revision` header; the UI takes the
  revision from schedule and override responses.
- There is no historical-usage endpoint yet. The mock provides seven days for
  the Stats tab; HTTP mode shows a clear "History unavailable" state.
- `limit_minutes: 0` means "no duration limit", not "locked". The emergency
  lock uses the time already used in whole minutes, which guarantees zero
  remaining for normal (> 1 minute) usage.

## Structure

```
src/
  app.css                       shared design system (ported from skeleton.html)
  env.ts                        explicit public/private environment contract
  lib/
    api/
      schema.d.ts               generated from docs/openapi.yaml
      types.ts                  wire aliases + TimetowerService interface
      mock.ts                   in-memory service with realistic fixtures
      http.ts                   typed API client
      index.ts                  service factory (mock vs http)
    domain/schedule.ts          status/window derivation matching Timetower
    state/dashboard.svelte.ts   runes-based app state and actions
    components/                 header, carousel, tabs, nav, shared panels
  routes/
    +page.svelte                app shell and tab routing
    api/v1/[...path]/+server.ts server-side Timetower proxy
```

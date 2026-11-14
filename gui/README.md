# Timetower Admin UI

A mobile-first admin console for Timetower: schedules, remaining daily time,
emergency locks, quick time extensions, weekly usage, and enforcer status.

The visual design follows [`docs/skeleton.html`](docs/skeleton.html) and the
behaviour is described in [`docs/SPEC.md`](docs/SPEC.md).

## Running

```sh
npm install

npm run mock   # in-memory fixtures, no backend required
npm run dev    # real Timetower API on the same origin
```

```sh
npm run check   # svelte-check + TypeScript
npm run build   # static build in build/
npm run preview # preview the production build
```

`npm run mock` starts Vite in `mock` mode. Every other mode uses the real API.
The production build uses `@sveltejs/adapter-static` and is served by the Go
backend from the same origin.

## API types and services

The wire types are generated from the canonical OpenAPI document at
[`../docs/openapi.yaml`](../docs/openapi.yaml):

```sh
npm run generate:api
```

This writes `src/lib/api/schema.d.ts`. Generation runs automatically before
`dev`, `mock`, `build`, and `check`, and the generated file is committed so a
plain `vite build` also works.

The service seam is `TimetowerService` in `src/lib/api/types.ts`:

| Implementation | File | Used when |
|---|---|---|
| `MockTimetowerService` | `src/lib/api/mock.ts` | `npm run mock` |
| `HttpTimetowerService` | `src/lib/api/http.ts` | all other modes |

`src/lib/api/index.ts` selects the implementation from the Vite mode. UI
components only see the interface, so swapping data sources does not touch the
views.

### Real backend

The GUI always calls same-origin relative URLs (`/api/v1/...`). The Go backend
serves the built UI and its assets, so there is no API URL or token in the
frontend configuration.

Admin requests use HTTP Basic auth:

- username: `admin`
- password: `TIMELORD_ADMIN_PASSWORD` from the backend `secrets.env`

The browser prompts for credentials when the API returns a Basic challenge and
reuses them for later requests. Use TLS for the admin origin; Basic auth sends
the password with every request.

The backend must also run with `TIMELORD_ENFORCER_KEY` for Enforcers and with
`-web` pointing at the built UI:

```sh
npm run build
./timetower ... -web gui/build
```

See [`../docs/Timetower.md`](../docs/Timetower.md).

### Endpoints used

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/users` | Configured users for the carousel |
| `GET /api/v1/schedule/{user}` | Policy, usage, remaining time, revision |
| `PUT /api/v1/override/{user}` | Quick adds, lock, and the override form |
| `GET /api/v1/enforcers` | Enforcer online status for the user indicators |

### Known API gaps

- There is no historical-usage endpoint yet. The mock provides seven days for
  the Stats tab; real mode shows a clear "History unavailable" state.
- `limit_minutes: 0` means "no duration limit", not "locked". The emergency
  lock uses the time already used in whole minutes, which guarantees zero
  remaining for normal (> 1 minute) usage.

## Structure

```
build/                         adapter-static output served by Timetower
src/
  app.css                       shared design system (ported from skeleton.html)
  lib/
    api/
      schema.d.ts               generated from docs/openapi.yaml
      types.ts                  wire aliases + TimetowerService interface
      mock.ts                   in-memory service with realistic fixtures
      http.ts                   same-origin API client (browser Basic auth)
      index.ts                  service factory (Vite mode)
    domain/schedule.ts          status/window derivation matching Timetower
    state/dashboard.svelte.ts   runes-based app state and actions
    components/                 header, carousel, tabs, nav, shared panels
  routes/
    +page.svelte                app shell and tab routing
```

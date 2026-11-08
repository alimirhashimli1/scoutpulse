# ScoutPulse — the full walkthrough, file by file

This is my preparation script for explaining ScoutPulse to a tech lead. It
complements [ARCHITECTURE.md](ARCHITECTURE.md), which is the high-level story.
This one goes file by file, concept by concept and tool by tool, and for every
piece it answers three questions:

1. **What is it?** (what the file does)
2. **How does it work?** (the mechanism, the code-level detail)
3. **Why is it like that?** (the decision, the alternative I rejected, the bug
   that taught me)

Each part ends with **"How I'd say it"**: a short version I can speak in an
interview without notes.

---

## Contents

- [Part 0 — How to present this in an interview](#part-0--how-to-present-this-in-an-interview)
- [Part 1 — The system in one picture](#part-1--the-system-in-one-picture)
- [Part 2 — One request, end to end](#part-2--one-request-end-to-end)
- [Part 3 — Repository layout and tooling](#part-3--repository-layout-and-tooling)
- [Part 4 — Shared libraries (`libs/`)](#part-4--shared-libraries-libs)
- [Part 5 — football-svc](#part-5--football-svc)
- [Part 6 — identity-svc](#part-6--identity-svc)
- [Part 7 — Frontend foundation (Angular 21, SSR)](#part-7--frontend-foundation-angular-21-ssr)
- [Part 8 — Frontend features](#part-8--frontend-features)
- [Part 9 — Testing, file by file](#part-9--testing-file-by-file)
- [Part 10 — Deployment and operations](#part-10--deployment-and-operations)
- [Part 11 — Concept glossary (concept → where it lives)](#part-11--concept-glossary-concept--where-it-lives)
- [Part 12 — Weak spots to own before they are found](#part-12--weak-spots-to-own-before-they-are-found)
- [Part 13 — Tech-lead questions, with answers](#part-13--tech-lead-questions-with-answers)

---

## Part 0 — How to present this in an interview

A tech lead usually wants three things in this order: **the shape**, **one
thing done well in depth**, and **how I think about trade-offs**. So:

1. **30 seconds**: what it is (below).
2. **2 minutes**: the system picture (Part 1), then trace one request through
   every layer (Part 2). Tracing a real request proves I know the code rather
   than the slogans.
3. **Let them steer.** Offer three deep dives and let them pick:
   - *Auth*: RS256 + JWKS, rotating refresh tokens with reuse detection, the
     single-flight refresh on the frontend.
   - *Data model*: the temporal model where history is the source of truth,
     and the transaction + row lock that keeps it consistent.
   - *SSR*: hybrid rendering, `ssrResource` and hydration, the CSP, and the
     Vercel adapter.
4. **Volunteer the weak spots** (Part 12) before they are found. Naming the
   limits of my own system is the strongest seniority signal there is.

**The 30-second version:**

> ScoutPulse is a Transfermarkt-style football database: competitions, clubs,
> players, transfer histories, market values and season stats, about 6,600 real
> players imported from an open dataset. On top of it are an animated 3D/2D
> tactics board, an editorial platform for analysis articles, and a trivia
> game. Technically it is two Go microservices (identity and football) sharing
> three libraries I wrote, PostgreSQL with one database per service, NATS for
> events, and an Angular 21 frontend that is server-side rendered for SEO.
> Locally it all runs in Docker Compose behind a Caddy gateway. In production
> the frontend is on Vercel, the services are on Railway and the databases are
> on Supabase.

**Numbers worth knowing** (measured from the repository):

| | |
|---|---|
| Go | ~23,000 lines across 6 modules |
| TypeScript | ~27,000 lines (frontend `src/`) |
| HTTP endpoints | ~70 in football-svc, 21 in identity-svc |
| Migrations | 11 football + 4 identity, all with working `down` files |
| Tests | 214 Go test functions (many table-driven), 137 frontend specs |
| History | 91 commits, May → November 2026 |
| Audit | 82 issues logged in `ISSUES.md`, 81 fixed |

---

## Part 1 — The system in one picture

```
                              Browser
                                 │  (one origin: same-origin /api/* calls)
            ┌────────────────────┴─────────────────────┐
            │ Production: Vercel                       │  Local: Caddy gateway :8000
            │  static assets → CDN                     │  (same path-prefix routing)
            │  /api/football/*  → rewrite → Railway    │
            │  /api/identity/*  → rewrite → Railway    │
            │  /.well-known/jwks.json → identity       │
            │  everything else → api/ssr.mjs (Angular) │
            └───────────┬──────────────────┬───────────┘
                        │                  │
        identity-svc (Go :8080)       football-svc (Go :8081)
        • holds RSA PRIVATE key       • holds only the PUBLIC key (JWKS)
        • users, sessions, OAuth,     • leagues, clubs, players, transfers,
          email verify, captcha         values, stats, notes, tactics,
        • publishes user.deleted,       articles, games
          user.role_changed  ──NATS──▶ subscribes, cleans up user-keyed data
                        │                  │
                  identity_db         football_db      (one Postgres, one DB +
                                                        one login role per service)
```

Inside every Go service the request path is identical:

```
HTTP → middleware chain → handler → service → repository → PostgreSQL
       RequestID           decode    validation  SQL, transactions,
       Recover             delegate  business    error translation
       Logging             encode    rules,      (pq error → apperr)
       CORS                          ALL authz
       Tracing, Metrics
       Recover (inner)
```

**How I'd say it:** "Two services because identity and football data change for
different reasons and carry different risk: only identity holds passwords and
the signing key. Every service has the same layering: handlers are transport
only, services own all business rules and *all* authorization, repositories own
SQL and translate database errors into a shared error taxonomy."

---

## Part 2 — One request, end to end

The best way to prove I know the system is to follow **"an editor records a
transfer"** from the click to the database row and the event.

1. **Browser.** The transfer form
   (`features/transfers/transfer-form.ts`) calls the injected
   `TRANSFER_WRITER` token. It never touches `HttpClient` directly. The
   implementation is `HttpTransferRepository` in
   `core/api/http/football-repositories.ts`, which extends `HttpRepository`
   and builds `/api/football/api/v1/transfers`. The URL is **root-relative**
   (`BROWSER_GATEWAY = ''`), so the browser only ever talks to its own origin.
2. **Interceptors** (registered in `app.config.ts`, outermost first):
   - `errorInterceptor` turns any `HttpErrorResponse` into a typed `ApiError`.
   - `authInterceptor` attaches `Authorization: Bearer <access token>` from
     the in-memory `SessionStore`. On a 401 it runs **one** shared refresh and
     retries.
   - `correlationIdInterceptor` adds `X-Request-ID`, so I know the id before
     the response arrives.
3. **Edge.** Locally, Caddy's `handle_path /api/football/*` strips the prefix
   and proxies to `football-svc:8081`. In production, a Vercel rewrite does
   the same thing to Railway.
4. **Middleware** (`libs/platform/server/server.go`, `RunWithOptions`):
   `RequestID` keeps the inbound id → outer `Recover` → `Logging` → `CORS` →
   `Tracing` (OpenTelemetry span named by route pattern) → `Metrics`
   (Prometheus) → inner `Recover`.
5. **Routing** (`football-svc/internal/handler/routes.go`). The route is
   registered with `protect("POST /api/v1/transfers", ...)`. `protect` wraps
   every non-GET route in a **per-account rate limiter** (burst 60, then 2/s)
   and then `auth.AuthMiddleware`, which validates the RS256 JWT and puts the
   `Claims` into the request context.
6. **Handler** (`transfer_handler.go`, `RecordTransfer`):
   `httpx.DecodeJSON` (1 MiB cap, unknown fields rejected, exactly one JSON
   object), then one service call, then `httpx.WriteJSON(201)`. No rules here.
7. **Service** (`service/transfer_service.go`, `RecordTransfer`):
   - `validateTransfer`: required fields, the date at most a year ahead, a
     known type, at least one club, the two clubs different, a non-negative
     fee, and "a free transfer cannot carry a fee".
   - Loads the player and runs `checkOrigin`: the stated `from_team_id` must
     equal the player's current club.
   - `authz.RequireEitherTeam(player.TeamID, t.ToTeam)`: an admin passes; an
     editor passes if they hold a grant for **either** club.
   - Attaches the season containing the transfer date, if one exists.
   - `repo.Record`, then **after the commit** publishes
     `football.player.transferred` through `SafePublisher`.
8. **Repository** (`repository/transfer_repository.go`, `Record`), in one
   transaction:
   - `SELECT team_id FROM players WHERE id=$1 FOR UPDATE` locks the player row
     and re-checks the origin *inside* the transaction (two concurrent
     transfers can no longer both pass the check).
   - `INSERT INTO transfers ... RETURNING id, created_at`.
   - `UPDATE players SET team_id=$1 WHERE ... AND NOT EXISTS (a newer
     transfer)`: the current club moves only if this is the player's latest
     move, so backfilling a 2015 transfer cannot change today's club.
   - Commit. Any Postgres error goes through `translate()` and becomes an
     `apperr` kind (unique → Conflict, FK → Invalid, `22P02` bad UUID →
     Invalid).
9. **Response.** On error, `httpx.WriteError` maps the kind to a status and
   writes `{error, code, request_id}`, logging the cause only for 5xx. On
   success the frontend shows the new transfer.
10. **Event.** NATS delivers the envelope (`id`, `subject`, `occurred_at`,
    `request_id`, `payload`) to any subscriber. If the broker is down, the
    request still succeeds: the event is dropped, logged and counted in
    `events_published_total{outcome="error"}`.

**How I'd say it:** "A transfer is the only way to move a player. It writes the
history row and updates the derived current club in one transaction, with the
player row locked and a guard so an older transfer can't overwrite a newer
state. The event goes out after the commit, so a broker failure can never undo
a fact."

---

## Part 3 — Repository layout and tooling

```
apps/
  football-svc/        core domain service (Go)
  identity-svc/        accounts and tokens (Go)
  frontend/            Angular 21 SSR app
  _template/           skeleton used by `make new-service`
libs/
  auth/                JWT issue/verify, JWKS, key handling
  db/                  pooled Postgres connection
  platform/            server, apperr, httpx, events, observability
deploy/
  gateway/             Caddyfile + Dockerfile
  postgres/            init-databases.sh (roles + databases)
  prometheus/          scrape config
tests/integration/     black-box tests against the whole compose stack
scripts/               PowerShell: dev setup, run, seed, backup/restore, tests
tools/played-for-both/ one-off dataset extractor for the trivia game
docs/                  architecture, frontend design, deploy and setup guides
```

### Root files

| File | What and why |
|---|---|
| `go.work` | **Go workspace** tying together six modules (two apps, three libs, integration tests). Locally, imports of `github.com/scoutpulse/libs/...` resolve straight from disk, so I can change a library and a service in one edit without publishing anything. |
| `go.work.sum` | Checksums for the workspace's combined dependency graph. |
| `Makefile` | The developer interface: `keys`, `up`, `down`, `logs`, `migrate`, `observability`, `test` (`-short`), `test-all`, `lint`, `fmt`, `tidy`, `clean`, `new-service`. The test and lint targets loop over `MODULES` and `exit 1` on the first failure, so a broken module fails the target instead of printing a note and carrying on. `new-service` copies `apps/_template`, substitutes the name and port with `sed`, and adds the module to `go.work` and the `MODULES` list. |
| `docker-compose.yml` | The full local stack (details below). |
| `.env.example` | Every variable, documented with *why* it exists. `.env` is gitignored. |
| `.golangci.yml` | **golangci-lint v2** config. It enables `bodyclose`, `errorlint`, `gosec`, `noctx`, `rowserrcheck`, `sqlclosecheck`, `revive`, `unparam`, `unconvert`, `misspell` on top of the v2 defaults. Two notable decisions are documented in the file: `govet shadow` is **off** (I read all 11 hits; every one was the idiomatic `if err := f(); err != nil`), and test files are exempt from `gosec`/`errcheck`/`noctx`. The migration to v2 was forced: v1 rejects `go 1.25.0` modules, which had *silently disabled* linting on three modules. |
| `.gitattributes` | Forces `eol=lf` (CRLF only for `.ps1`). This fixed a real CI outage: Windows `autocrlf` had committed CRLF into 103 files, `gofmt -l` and `prettier --check` failed on them, and because those steps run first, build/test never ran. The whole pipeline looked red for a line-ending setting. |
| `.dockerignore`, `.gitignore`, `.geminiignore` | Keep build contexts and the repo clean. |
| `.mcp.json` | Configures the Vercel MCP server for the AI assistant (used for deployment inspection). |
| `.github/workflows/main.yml` | CI (Part 10). |
| `PROJECT_SPEC.md`, `ISSUES.md`, `IMPROVEMENTS.md`, `ONBOARDING_PROGRESS.md` | The spec, the audit register (82 issues by priority P0–P5, 81 fixed), the improvement backlog and onboarding notes. |

### `docker-compose.yml`, service by service

- **`postgres`** (`postgres:16-alpine`): *one* instance. `init-databases.sh`
  is mounted into `docker-entrypoint-initdb.d` and creates one database and
  one login role per service. The port is bound to `127.0.0.1` only, because
  the example credentials are working ones. The health check is `pg_isready`.
- **`nats`** (`nats:2.10-alpine`): `--jetstream --http_port 8222`, loopback
  only (no auth on NATS, so an exposed port would let anyone publish events).
  The health check hits `/healthz`.
- **`migrate-identity` / `migrate-football`** (`migrate/migrate:v4.17.1`):
  **one-shot jobs** that run `migrate up` against a `schema_migrations` table,
  so they only apply what is pending. This separation means a schema change
  never requires wiping the volume.
- **`identity-svc`, `football-svc`**: built from the repo root (the Dockerfiles
  need `libs/`). They **wait on conditions**, not just on start order:
  `postgres: service_healthy`, `migrate-*: service_completed_successfully`,
  `nats: service_healthy`. Only identity gets `JWT_PRIVATE_KEY`; football gets
  only `JWT_PUBLIC_KEY`. `${JWT_PRIVATE_KEY:?run "make keys"...}` makes
  compose fail with a helpful message if the keys are missing.
- **`gateway`** (`caddy:2.8-alpine`): waits for both services to be
  *healthy*, otherwise it would answer 502 while they were still connecting.
  It is the only port published on all interfaces (`8000`).
- **`frontend`**, **`prometheus`**, **`jaeger`**: behind compose **profiles**
  (`frontend`, `observability`), so the default stack stays small.

### `deploy/gateway/Caddyfile`

```
:{$PORT:8000} {
    handle /.well-known/jwks.json { reverse_proxy identity-svc{$INTERNAL_SUFFIX}:8080 }
    handle_path /api/identity/*   { reverse_proxy identity-svc{$INTERNAL_SUFFIX}:8080 }
    handle_path /api/football/*   { reverse_proxy football-svc{$INTERNAL_SUFFIX}:8081 }
    handle                        { reverse_proxy frontend{$INTERNAL_SUFFIX}:4000 }
}
```

- `handle_path` **strips** the prefix, so each service only knows
  `/api/v1/...`.
- `{$INTERNAL_SUFFIX}` is empty in compose and `.railway.internal` on Railway.
  Hard-coding either one breaks the other, and it shows up as a 502 from a
  gateway that looks healthy.
- `admin off` disables Caddy's admin API.
- **Rate limiting is deliberately not here**: Caddy's rate-limit module is a
  third-party plugin missing from the official image, so it lives in the
  services, where it is unconditional and unit-testable.

### `deploy/postgres/init-databases.sh`

Creates `identity_db`/`identity_user` and `football_db`/`football_user`.
Two details a tech lead will like:

- **No SQL injection from the shell.** Values are passed with `psql -v` and
  quoted with `format('CREATE ROLE %I LOGIN PASSWORD %L', ...)` plus `\gexec`.
  The script runs as superuser, so a password containing a quote must not be
  able to change the statement.
- **A `\gexec` subtlety is documented.** The `SELECT` lines have no trailing
  `;` on purpose: a `;` sends and clears the query buffer, `\gexec` then has
  nothing to run, the role is never created, and the failure shows up later as
  "database does not exist". The script also verifies afterwards that the
  database exists and fails loudly if not.

### `deploy/prometheus/prometheus.yml`

One scrape job per service, plus NATS's `/varz`. All services expose the same
metric names because they come from one library.

### `apps/_template/`

`main.go.tmpl`, `go.mod.tmpl` and `Dockerfile.tmpl` contain a service that
already has token verification, optional events, tracing, metrics, `/health`
and graceful shutdown. `make new-service NAME=x PORT=8082` turns it into a
module. **Why:** so every new service starts with the same platform wiring
rather than a hand-copied approximation of the last one.

**How I'd say it:** "It's a monorepo with a Go workspace, so libraries are
shared from disk in development, but each Docker build sets `GOWORK=off` and
builds from the service's own `go.mod` with `replace` directives, so images are
reproducible. Compose uses health checks and one-shot migration jobs, so
startup order is about readiness, not just 'the container started'."

---

## Part 4 — Shared libraries (`libs/`)

The libraries exist because once there were two services, I was about to copy
the same 500 lines a second time. Extracting them made a third service cheap.

### 4.1 `libs/platform/server`

**`server.go`: the HTTP bootstrap.**

- Constants: `ReadHeaderTimeout=5s`, `ReadTimeout=15s`, `WriteTimeout=30s`,
  `IdleTimeout=60s`, and `ShutdownTimeout = WriteTimeout + 5s`.
  - `ReadHeaderTimeout` closes the **Slowloris** hole: a bare
    `http.ListenAndServe` has no header timeout, so a client sending one byte
    every few seconds holds a connection forever.
  - The shutdown window is deliberately **longer** than the write timeout, so
    a slow request that is still inside its write budget can finish during a
    drain.
- `RunWithOptions(name, addr, handler, opts)` builds the middleware chain
  `RequestID → Recover → Logging → CORS → [caller's middleware] → Recover`.
  `Recover` appears **twice on purpose**: the outer one catches a panic even in
  the metrics/tracing middleware; the inner one catches application panics
  *below* observability, so they are still counted and traced like any 500.
- **Graceful shutdown**: `signal.NotifyContext(SIGINT, SIGTERM)` (what a
  container runtime sends), `srv.Shutdown(ctx)` stops accepting connections
  and drains in-flight requests, then the `OnShutdown` hooks run (for example
  flushing traces), so telemetry for the last requests isn't lost.
- `Health(name)` is the liveness handler.

**`middleware.go`.**

- `Chain`: applies middleware so the first entry is the outermost.
- `RequestID`: keeps an inbound `X-Request-ID` or generates 16 random bytes as
  hex; stores it in the context and echoes it in the response. Keeping the
  inbound one means a chain of calls across services shares one id in logs
  and traces.
- `statusRecorder`: wraps `ResponseWriter` to capture the status and byte
  count.
- `Logging`: one structured `slog` line per request (method, path, status,
  bytes, duration, request id). **Successful** `/health` and `/metrics` hits
  are skipped (Docker health checks and Prometheus scrapes produce thousands
  of identical lines); a *failing* probe is still logged.
- `Recover`: logs the panic with stack and request id, and returns the
  **same JSON error shape** as every other error (`code: "internal"`), so a
  client needs one parser.
- `CORS`: allowed origins come from `CORS_ALLOWED_ORIGINS`. **Unset means deny
  all, not allow all**: a misconfigured deployment should fail visibly in the
  browser console, not silently be permissive. With a specific origin it sets
  `Vary: Origin` (so caches don't serve one origin's response to another) and
  `Allow-Credentials`. Preflight allows `Authorization`, `Content-Type`,
  `X-Session-Cookie`, `X-Forwarded-Prefix` and `X-Request-ID`.

**`ratelimit.go`: an in-process token bucket.**

- Each key has a bucket `{tokens, lastSeen}`. On each request it refills by
  `elapsed × rate`, caps at `burst`, then spends one token or refuses with
  `Retry-After`.
- `sweepLocked` deletes buckets idle for more than 10 minutes, at most once a
  minute, so many distinct clients can't make every request a full map scan
  or grow memory forever.
- `Middleware` keys by IP; `MiddlewareBy(keyFn)` keys by anything (the
  football service keys writes by **account**).
- `Blocked(key)` + `Record(key)` build a limiter that **only counts failures**:
  login checks `Blocked` first and calls `Record` only when the password is
  wrong, so a user who signs in correctly never spends their budget.
- `ClientIP` reads the IP from a source chosen **once at startup** from
  `CLIENT_IP_HEADER` (`""` = TCP peer, `X-Real-IP` on Railway,
  `X-Forwarded-For` first entry behind Caddy). It is not guessed per request,
  because a header any caller can send is a rate-limit key any caller can
  change. If the configured header is missing, it falls back to the peer
  address, never to another header.

### 4.2 `libs/platform/apperr`: the error taxonomy

- `Kind`: `Internal` (the zero value, so an unclassified error is internal by
  default), `Invalid`, `Unauthorized`, `Forbidden`, `NotFound`, `Conflict`,
  `RateLimited`.
- `Error{Kind, Message, Cause}`: `Message` is **client-safe**, `Cause` is
  **logged only**. It implements `Unwrap`, so `errors.As` works through
  wrapping.
- `KindOf(err)` and `Message(err)` walk the chain; anything that didn't come
  from this package is `Internal` with the message "internal server error".
  **That is the mechanism that stops SQL or driver text from ever reaching a
  response body.**

### 4.3 `libs/platform/httpx`

- `StatusFor(kind)`: the **only** kind → HTTP status mapping in the codebase.
- `WriteJSON`: sets the content type and status; an encoding failure after the
  status is committed can't be reported to the client, so it is logged.
- `WriteError`: kind → status, logs 5xx with the request id, writes
  `{error, code, request_id}`.
- `DecodeJSON`: `MaxBytesReader` (1 MiB), `DisallowUnknownFields`, and
  `dec.More()` rejects trailing content. Without the last check,
  `{"role":"user"}{"role":"admin"}` decodes the first object and silently
  discards the second: a smuggled payload goes unreported.
- `query.go`: `QueryInt`, `QueryBool` (returns `*bool`, so "absent" differs
  from "false") and `QueryString`. A malformed value is a 400, not a silent
  default, so typos surface.

### 4.4 `libs/platform/events`: the event contract and NATS

**`events.go`.**

- Subjects are named `<service>.<entity>.<past-tense fact>`:
  `football.player.transferred`, `identity.user.deleted`, and so on. Events
  describe something that has already happened, so a consumer can only react,
  never reject.
- `Envelope{ID, Subject, OccurredAt, RequestID, Payload json.RawMessage}`: the
  UUID allows deduplication, and the request id ties the event back to the
  HTTP request that caused it.
- `Publisher` and `Subscriber` interfaces. `Subscribe` takes a **queue
  group**: replicas of one app share a group, so exactly one replica handles
  each event (competing consumers, not a broadcast).
- Typed payload structs (`PlayerTransferred`, `UserDeleted`, ...).

**`nats.go`.**

- `Connect` reconnects forever (`MaxReconnects(-1)`, 2 s apart) and logs
  disconnects and reconnects.
- `PublisherFromEnv`: no `NATS_URL` → `NopPublisher`. A URL that is set but
  **unreachable** → also `NopPublisher` plus an error log, rather than refusing
  to start. Read paths publish nothing and must not depend on the broker.
- `Publish` wraps the payload in an envelope with a fresh UUID and the request
  id from the context.
- `Close` uses `Drain()`, which flushes buffered messages before closing.
- `Subscribe` decodes the envelope; a malformed message or a failing handler
  is **logged, not fatal**, so the subscription keeps delivering.
- `SafePublisher` wraps any publisher, **always returns nil**, and records
  `events_published_total{service,subject,outcome}`. The write it describes is
  already committed; telling the client their transfer failed because the
  broker hiccupped would be a lie.

### 4.5 `libs/platform/observability`

**`metrics.go`.**

- `http_requests_total{service,method,route,status}`,
  `http_request_duration_seconds` (buckets from 5 ms to 10 s),
  `http_requests_in_flight`, `events_published_total`.
- The label is `r.Pattern` (Go 1.22+ fills in the matched pattern, such as
  `GET /api/v1/players/{id}`), **not the raw path**: raw paths would create a
  time series per player id (a cardinality explosion).
- `Handler()` guards `/metrics`: with `METRICS_TOKEN` set it requires
  `Bearer <token>`, compared with `subtle.ConstantTimeCompare`; with no token
  on Railway (detected by `RAILWAY_PROJECT_ID`) it answers 404; locally it is
  open for the compose Prometheus. The reason: a Railway service has a public
  domain that bypasses the gateway, and metrics leak request volumes.

**`tracing.go`.**

- `InitTracing`: always installs the W3C `TraceContext` + `Baggage`
  propagators (so an upstream trace id passes through even when this process
  isn't exporting). With no `OTEL_EXPORTER_OTLP_ENDPOINT` it is a no-op.
  Otherwise: an OTLP/HTTP exporter, a batcher, resource attributes, and a
  **parent-based ratio sampler** (`OTEL_TRACES_SAMPLER_ARG`), so a trace
  sampled at the edge stays sampled through every service.
- `Tracing` middleware uses `otelhttp`, names spans `METHOD pattern`, and
  filters out `/metrics` and `/health`.

### 4.6 `libs/db/db.go`

- `Config` from `DB_*` variables, including `DB_SSLMODE` (default `disable`
  for compose; `require` in production).
- `DSN()` is built with `net/url`, **not string formatting**, so a password
  with a space, quote or `&` is escaped instead of truncating the DSN or
  injecting a connection parameter.
- Pool defaults: `MaxOpenConns=25`, `MaxIdleConns=25`, lifetime 30 min, idle
  5 min. Postgres defaults to `max_connections=100` *shared by every client*,
  and `database/sql`'s default of unlimited open connections would exhaust it
  under load. Idle = open avoids paying a new handshake per request (the
  driver default is 2 idle).
- `withDefaults()` applies those limits even to a `Config` built by hand
  (tests, tools), because zero means "unlimited" to `database/sql`.
- `Connect` pings with a 10-second timeout, so a dead database fails fast at
  startup instead of hanging.

### 4.7 `libs/auth`: tokens

**`auth.go`.**

- `AccessTokenTTL = 15 min`, `RefreshTokenTTL = 30 days`. Access tokens are
  stateless and can't be recalled, so their lifetime *is* the leak window.
  Long sessions ride on the revocable refresh token.
- `Claims{UserID, Role, Username, RegisteredClaims}`. **No team grants in the
  token.** They used to be there as `managed_team_ids`, which froze
  permissions for the token's lifetime (a revoked grant kept working until
  expiry). `Username` is display data only, never an authorization input.
- `GenerateToken`: sets `jti` (random id), `sub`, `iss`, `iat`, `nbf`, `exp`,
  and the `kid` header (so a verifier can pick the right key during
  rotation).
- `ValidateToken`: `jwt.WithValidMethods(["RS256"])` **pins the algorithm**
  (prevents algorithm confusion, such as `alg: none` or HS256 signed with the
  public key), plus `WithIssuer` and `WithExpirationRequired`. If the `kid` is
  unknown, it triggers **one** rate-limited JWKS refetch and retries, which is
  how a key rotation reaches verifiers without a redeploy.
- `keyForToken`: picks the key by `kid`. A token without a `kid` is accepted
  only if exactly one key is installed.
- `AuthMiddleware`: `Authorization: Bearer <t>`. The scheme is compared
  case-insensitively (RFC 7235), and errors are JSON in the same shape as
  `httpx` (duplicated on purpose so `libs/auth` doesn't depend on `platform`).
- `NewRefreshToken`: 32 random bytes, base64url. **Opaque, not a JWT**: it
  means nothing without its server-side row, which is what makes it revocable.

**`keys.go`.**

- Global key state behind a `sync.RWMutex`: one optional signing key, plus a
  map of `kid → public key` (several during a rotation).
- `MinRSAKeyBits = 2048`, checked on **verification** keys too. That matters
  more than the signing key: a factorable verification key lets an attacker
  mint tokens this service will accept, and verification keys arrive from
  outside (env or JWKS).
- `KeyID` = base64url(SHA-256(DER of the public key)): derived from the key,
  so it can never disagree with it.
- `LoadSignerFromEnv` (identity only) vs `LoadFromEnv` (everyone else; a
  pinned `JWT_PUBLIC_KEY` or `JWKS_URL`).
- `normalisePEM` accepts keys that arrive quoted or with literal `\n`
  sequences (how hosting dashboards mangle multi-line env vars). It cost an
  evening to tell "bad key" from "bad transport".
- It accepts both PKCS#1 and PKCS#8 / PKIX.

**`jwks.go`.**

- `PublicJWKS` and `JWKSHandler` publish `{kty, use, alg, kid, n, e}` with
  `Cache-Control: max-age=300`.
- `LoadJWKS` requires **https** unless the host is loopback, private, or an
  in-cluster name (`.internal`, `.svc`, a single-label compose name). Over
  plain http the key set could be swapped in transit, and these keys decide
  which tokens are trusted.
- `RefreshJWKS` is rate-limited to once per 30 s, so an attacker spraying
  random `kid`s can't make the service fetch per request.
  `StartJWKSRefresh` also polls every 5 minutes in the background.
- Keys of an unknown type are skipped rather than rejecting the whole
  document; the RSA size floor applies to fetched keys too.

**`cmd/genkeys/main.go`**: generates a 2048-bit pair in Go and prints
`.env` lines with escaped newlines. It exists because `make keys` used to shell
out to `openssl`, which a default Windows install doesn't have.

**How I'd say it:** "The platform library is a reusable service chassis:
hardened timeouts, a correlation id, structured logs, panic recovery, CORS that
fails closed, a token-bucket limiter, a typed error taxonomy that keeps driver
detail out of responses, optional events and optional telemetry. The rule
throughout is that *optional infrastructure must never take down a request*:
no broker means a no-op publisher, and no collector means no-op tracing."

---

## Part 5 — football-svc

The core domain service: everything football, plus tactics boards, articles and
games. Module `github.com/scoutpulse/football-svc`, port 8081.

### 5.1 `main.go`: wiring by hand

The order is the architecture:

1. A JSON `slog` logger, set as the default.
2. `auth.LoadFromEnv()` (**fail fast**: without a verification key, every
   protected route would reject every request) and `auth.StartJWKSRefresh`.
3. `db.ConnectFromEnv()`.
4. `events.PublisherFromEnv` → wrapped in `NewSafePublisherFor("football-svc")`.
   A type assertion `publisher.(events.Subscriber)` checks whether a real NATS
   connection exists to subscribe with.
5. **15 repositories** (Postgres implementations behind interfaces).
6. **One `Authorizer`** shared by every service, so the grant cache is shared
   and one invalidation reaches all of them.
7. `IdentityEventConsumer.Register(...)` if subscribing is possible; otherwise
   a warning that grants won't be cleaned up when an account is deleted.
8. **15 services**, each receiving exactly the repositories it needs.
9. The games dataset is parsed (`playedforboth.Load()`) **before serving
   anything**, so a corrupt embedded file stops the boot instead of breaking
   the game in browsers.
10. A `handler.Handlers` struct (named fields, so a miswired dependency is a
    compile error, not a swapped positional argument).
11. `InitTracing`, then `RegisterRoutes`, `/metrics`, and
    `server.RunWithOptions(..., Tracing, Metrics, OnShutdown: shutdownTracing)`.

**Why no DI framework:** explicit wiring is readable top to bottom, and a
missing dependency is a compile error. At this size a container adds magic
without saving work.

### 5.2 `Dockerfile` (the same pattern for identity-svc)

- Stage 1: `golang:1.25-alpine`, `COPY . .` (the repo root, so the `replace
  => ../../libs/...` directives resolve), `ENV GOWORK=off` (build from the
  module's own `go.mod`/`go.sum`, which is reproducible), `go mod download`,
  then `CGO_ENABLED=0 go build -ldflags="-s -w"` (a static, stripped binary).
- Stage 2: `alpine:3.19`, `adduser -D scoutuser`, `USER scoutuser`, copy the
  binary only. **Non-root**, small, and no toolchain in production.

### 5.3 `go.mod`

`replace` directives point `libs/auth`, `libs/db` and `libs/platform` at
`../../libs/...`. Direct dependencies: `sqlx`, `lib/pq`, `google/uuid`,
`testify`, `testcontainers-go`. Everything else (NATS, OTel, Prometheus, jwt)
comes in transitively through the libraries.

### 5.4 `internal/domain/`: types with no dependencies on HTTP or SQL

| File | What it holds, and the interesting decision |
|---|---|
| `models.go` | `League` (a *competition*, with `CompetitionType`: league, domestic cup, international cup, super cup), `Season`, `Team`, `TeamSeason`, `Player`, `PlayerNote`, `Transfer` (+ `TransferType`: permanent, loan, loan_return, free, youth_promotion, released, retired), `PlayerSeasonStat`, `MarketValue`, `Coach`, `CoachSpell` (+ `SpellRole`). Every enum has a `Valid*` function matching a database `CHECK`. Optional fields are pointers, because **nil and zero are different facts** (`Fee == nil` means *undisclosed*; a free transfer is `type=free, fee=0`; a nil scouting percentage means "not recorded", not 0%). `Player.Age()` handles a birthday that hasn't happened yet this year. |
| `money.go` | `Minor int64`: money in **minor units** (cents). `MarshalJSON` emits an integer. `UnmarshalJSON` accepts `2500` or `"2500"` (JavaScript loses precision above 2^53, so clients may send strings), **rejects decimals** rather than guessing whether `1.5` meant 1 or 2 cents, and treats `null` as "not supplied". `Money` pairs an amount with a currency (default EUR). |
| `page.go` | `Page{Limit, Offset}`, `NewPage` clamps (default 25, max 100, negative offset → 0). `FetchLimit()` = limit + 1: **fetch one extra row to know if there's a next page, without a `COUNT(*)`**. `ListResult[T]` (a Go generic) trims the sentinel row, sets `has_more`, and turns `nil` into `[]`, so clients never see `null`. |
| `stringlist.go` | `StringList` for Postgres `TEXT[]`: implements `sql.Scanner`/`driver.Valuer` (a bare `[]string` fails at *run time* on the first row) and always marshals as `[]`, never `null`. |
| `search.go` | `SearchKind` and `SearchResult{kind, id, name, subtitle, team_id, rank}`: just enough to render a dropdown row and navigate. |
| `tactics.go` | `TacticBoard` (the internal `ID` is `json:"-"`; the **slug** is the only public address) and `TacticSummary` (no document; the step count is computed in SQL). |
| `articles.go` | `ArticleStatus` (draft, in_review, changes_requested, published), `Article`, `Live(now)` = published **and** `publish_at <= now`, `ForReaders()` strips the reviewer and review note (a desk conversation, not part of the piece), `ArticleSummary`, `ArticleDeskFilter`. |

### 5.5 `internal/handler/`: transport only

- **`routes.go`**: every route uses Go 1.22+ `ServeMux` patterns
  (`"GET /api/v1/players/{id}"`, read with `r.PathValue("id")`). **Reads are
  public; writes go through `protect`**, which wraps a non-GET route in the
  per-account limiter (keyed `user:<id>` from the claims, falling back to IP),
  then `auth.AuthMiddleware`. Authentication only proves *who* you are; *what*
  you may do is decided in the service. Ordering matters in a few places:
  `GET /seasons/current` is registered before `/seasons/{id}`. Some GETs are
  protected too: editor lists (access-control data), `notes/mine`, and the
  tactics and desk lists.
- **`common.go`**: `pageFrom(r)` (limit and offset → `domain.Page`) and
  `idsFrom(r)` (a comma-separated `?ids=`, blanks dropped so a trailing comma
  isn't a malformed UUID, capped at 100 ids as a DoS guard).
- **One file per resource**: `league_`, `season_`, `team_`, `team_season_`,
  `team_editor_`, `coach_`, `coach_spell_`, `player_`, `player_note_`,
  `player_stats_`, `market_value_`, `transfer_`, `search_`, `tactic_`,
  `article_`, `game_handler.go`. Each method is the same three steps: decode
  (`DecodeJSON`, path values, query filters) → one service call → `WriteJSON`
  or `WriteError`. On updates the **path id overrides any id in the body**, so
  a body can't redirect the write. `TeamHandler` also takes the player and
  coach services so `GET /teams/{id}` can embed the squad (bounded at 100) and
  the coach.
- **`game_handler.go`** serves the trivia dataset without touching the
  database. At construction it computes the body, a **gzip copy**
  (`BestCompression`, about a third of ~430 KB) and a **strong ETag**
  (SHA-256). Per request it copies bytes out: `Cache-Control: public,
  max-age=86400`, `Vary: Accept-Encoding`, a separate `-gz` ETag for the
  compressed variant (a strong ETag names exact bytes), and `304 Not
  Modified` on `If-None-Match`. It parses `Accept-Encoding` q-values
  properly (`gzip;q=0` means no).

### 5.6 `internal/service/`: business rules and all authorization

**`football_service.go`: the `Authorizer`.**

- `RequireAdmin`, `RequireAuthenticated`, `RequireTeam(teamID)`,
  `RequireTargetTeam(*teamID)` (a nil club means admin-only, because no club
  grant can cover a free agent), and `RequireEitherTeam(current, target)` for
  transfers and coaching spells.
- `managesTeam` checks a **5-second TTL cache** keyed `userID\x00teamID`
  before asking `team_editors`. The cache is swept past 1,024 entries.
  `InvalidateUser` / `InvalidateTeam` clear entries immediately when a grant
  changes, so revocation is immediate on this instance and at most 5 seconds
  stale anywhere else.
- **Why grants aren't in the JWT:** I tried that first. A new grant needed a
  fresh login, and a revocation couldn't take effect until the token expired.
  Grants are football-domain data, so they live in the football database.

**`errors.go`**: shared `ErrForbidden`, `ErrUnauthorized`, `ErrNotFound`
values that carry `apperr` kinds.

**Per-domain services** (one interface plus a private struct each):

| Service | Key rules |
|---|---|
| `league_service.go` | Admin-only writes; validates the name, country and a valid `competition_type`. |
| `season_service.go` | Admin-only writes; end after start; `checkNoOverlap` turns the database's exclusion constraint into a readable "overlaps season 2025/26" conflict. `CurrentSeason` finds the season containing today. |
| `team_service.go` | Create and delete are admin-only. **Update**: an editor with a grant may edit, but **may not change `league_id`** (moving a club between competitions is a claim about standing). The stored value is *preserved* rather than the request rejected, so a GET→PUT round trip doesn't fail for echoing a field. Delete invalidates the cached grants for that club and publishes `TeamDeleted` (players become free agents through `ON DELETE SET NULL`). |
| `team_season_service.go` | `Enter`: `RequireTeam`, then resolves the team, season and league, so a bad id gives a precise 404 instead of one opaque FK violation. `Withdraw` is admin-only (removing a club from a competition it played in rewrites history). |
| `team_editor_service.go` | Grant and revoke are admin-only. Grant validates that `user_id` is a UUID (users live in another database, so there's no FK to check against), checks the team exists, records `granted_by`, then `InvalidateUser`. |
| `coach_service.go` / `coach_spell_service.go` | A spell's end date is after its start; its start is at most a year ahead; a known role (default head_coach). Authorized by either club, like transfers. Publishes `CoachAppointed`. |
| `player_service.go` | `validatePlayer`: height 100–250 cm, squad number 1–99, no future birth date, contract dates ordered, at most 3 nationalities and 6 secondary positions, the main position not repeated in the secondary list, at most 12 strengths/weaknesses of 200 **runes** each (characters as the author counts them, not bytes), percentages 0–100. Lists are *tidied* (trimmed, blanks dropped) rather than rejected. **`UpdatePlayer` preserves `team_id`, `market_value_minor` and `currency`**: those are derived state, and the only way to move a player is a transfer. |
| `transfer_service.go` | Covered in Part 2. `UpdateTransfer` can only change type, fee, currency and season; the player, clubs and date come from the stored row, because they're what the current club is derived from. `DeleteTransfer` is admin-only and deliberately does **not** rewind the player's club: guessing the previous club from a deleted row is often wrong. |
| `market_value_service.go` | **Admin-only**, because a valuation is an editorial judgement; otherwise every editor could inflate their own squad. No future dates. Publishes `PlayerValueChanged` with the previous and new value. |
| `player_stats_service.go` | Read-only. Stats come from the importer; there's no match data to check a hand correction against. |
| `player_note_service.go` | **Any signed-in member** may write one note per player. The author name comes **from the token**, never the body (otherwise anyone could sign as someone else). A unique-constraint conflict becomes "you've already written a note; edit that one". Edit is author or admin (moderation usually means removing one sentence); delete is the same. |
| `search_service.go` | Public. At least 2 characters (a single character matches a large fraction of the data and costs an index scan); validates `kind`. |
| `tactic_service.go` | Any member may create boards. Slugs are 12 characters from an alphabet without look-alikes (`0/O`, `1/l/I` removed), about 70 bits of randomness, so boards can't be found by guessing. Collisions are handled by **retrying the insert on a unique conflict** (only the constraint can settle a race; check-then-insert can't). The content must be a JSON object up to 512 KB; at most 500 boards per account (accounts are free, so the table would otherwise be open-ended storage). Only the owner may update; owner or admin may delete. `List` is forced to the caller's own boards unless they're an admin. |
| `article_service.go` | The editorial workflow (below). |
| `article_body.go` | Server-side document validation (below). |
| `identity_events.go` | The NATS consumer (below). |

**`article_service.go`: the workflow.**

- Who may do what: anyone reads *live* articles; editors and admins write; an
  editor sees and edits only **their own**, and only in draft or
  changes-requested; an admin sees and edits everything, publishes, schedules
  and sends back with a note. An admin's own "submit" publishes directly.
- `reachable()`: someone else's article answers **404, not 403**, so an editor
  can't learn which ids exist.
- `Submit` validates readiness (a title and at least one word), normalises
  `publish_at` (rejects more than 366 days ahead as a likely typo; a time in
  the past means now). Editor: → `in_review`. Admin: `publish()` keeps the
  writer's requested time if it is still ahead, and sets `first_published_at`
  once.
- **Slugs**: `slugWords(title)` NFKD-normalises and strips accents
  ("Atlético's low block" → `atleticos-low-block`), plus a 6-character random
  suffix. The slug follows the title **until first publication, and never
  moves after**, so shared links keep working.
- Tags are lower-cased and de-duplicated (at most 8, 32 characters each).
  Limits: title 160, subtitle 300, body 512 KB, 1,000 articles per author.
- `now` and `newSuffix` are injectable fields, so tests control the clock and
  can force a slug collision.

**`article_body.go`: stored-XSS defence.**

- The body is **ProseMirror JSON**, not HTML. `checkArticleBody` walks the
  whole tree: every node type must be on an allow-list (`paragraph`,
  `heading`, lists, `blockquote`, `figure`, `tacticBoard`, `video`, ...), as
  must every mark (`bold`, `italic`, `strike`, `underline`, `link`).
- Bounds: depth 12, 20,000 nodes, 200,000 runes of text.
- Headings are level 2–4 only (the title is the page's h1).
- `SafeLinkURL`: http(s) with a host, `mailto:`, or a same-site path, but
  **not** `//host` or `/\host` (protocol-relative tricks), and never
  `javascript:` or `data:`. `SafeImageURL`: https only (http would be mixed
  content).
- Embeds: a tactics board must have a valid slug; a video must be YouTube
  (11-character id regex) or Vimeo (6–12 digits).
- It also **derives** the excerpt (the first paragraphs up to 280 characters,
  cut at a word) and reading time (230 words per minute). Derived on the
  server, so a client can't lie about them.

**`identity_events.go`: cross-database cleanup.**

- Subscribes to `identity.user.deleted` and `identity.user.role_changed` in
  queue group `football-svc`.
- On user deleted: revoke all editor grants → `InvalidateUser` → delete their
  tactics boards → delete their **unpublished** articles (published ones stay:
  readers have linked to them and the byline was captured at write time).
- A malformed payload is **dropped** with a log (it won't become valid on
  redelivery). A real database failure **returns an error** so it can be
  retried. Every step is idempotent, so a retry after partial success is
  harmless.
- On role changed: clears cached decisions, because the cache combines the
  grant with the role from the token.
- **Why it exists:** `team_editors.user_id` can't have a foreign key to
  `users`, which lives in another database. Without this, grants would outlive
  the account, and if an id were ever reissued they would apply to someone
  else.

### 5.7 `internal/repository/`: SQL and error translation

- **`errors.go`**: `translate(entity, err)` maps `sql.ErrNoRows` → NotFound,
  `23505` unique → Conflict, `23503` FK → Invalid, `23502` not-null →
  Invalid, `23514` check → Invalid, and **`22P02` invalid text
  representation** (a malformed UUID) → Invalid. That last one matters:
  without it, `/players/not-a-uuid` was a 500 with an error in the logs for a
  plain bad request. `affected()` turns "0 rows affected" on an
  update/delete into NotFound, so writes to a missing id don't silently
  succeed.
- **Dynamic filters without SQL injection** (`player_repository.go`,
  `transfer_repository.go`): a `placeholder(v)` closure appends the value to
  `args` and returns `$N`, so the WHERE clause and the argument list **can't
  drift apart**. Only constant SQL fragments are concatenated. Lists are
  ordered `name, id` / `transfer_date DESC, id`: the `id` tiebreak makes
  paging stable. `nationalities @> ARRAY[$n]` (containment) handles dual
  nationals; `id = ANY($n)` serves the batch `?ids=` lookup that removed an
  N+1 on the transfer feed.
- **`player_repository.go` `Create`**: one transaction inserts the player
  **plus** an opening transfer (from nowhere to the club) **plus** an opening
  valuation, so a new player never has a club without a history row behind
  it. `Update` deliberately leaves out `team_id` and the market value.
- **`transfer_repository.go` `Record`**: the `FOR UPDATE` lock + `NOT EXISTS`
  guard (Part 2). `Update` changes only the descriptive columns.
- **`market_value_repository.go`**: `Record` is an upsert,
  `ON CONFLICT (player_id, valued_on) DO UPDATE` (one valuation per player per
  day), then updates the derived current value only if no **later** valuation
  exists. `Delete` is scoped to the player in the path, then **recomputes**
  the current value with a `LEFT JOIN LATERAL ... ORDER BY valued_on DESC
  LIMIT 1` and `COALESCE` (deleting the only valuation gives 0).
- **`search_repository.go`**: `ToTSQuery` keeps only letters and digits
  (`to_tsquery` takes an *expression language*, so raw input could cause a
  syntax error or a pathological query), appends `:*` for prefix matching
  ("mess" finds Messi), and joins with `&` (more words narrow the results).
  Four `SELECT`s with identical columns are joined by `UNION ALL`; the rank is
  `ts_rank(document, query) + similarity(name, raw input)` (full text plus a
  trigram tiebreak), ordered by `rank DESC, name, id`.
- **`tactic_repository.go`**: the summary query computes
  `jsonb_array_length(content->'steps')` guarded by `jsonb_typeof`, so a list
  doesn't ship 50 documents. `($1::uuid IS NULL OR owner_id = $1)` is an
  optional filter in one statement. JSON is passed as a `string`, because
  `lib/pq` sends `[]byte` as `bytea`, which Postgres won't read as JSON.
- The other repositories follow the same pattern: a column list constant,
  `GetContext`/`SelectContext`, `translate`, `affected`.

### 5.8 `migrations/`: the schema story in 11 steps

Every migration uses `IF NOT EXISTS` / `DROP ... IF EXISTS` and has a working
`down`. CI runs **up → down → up**.

| # | What, and the reason |
|---|---|
| 000001 | The first, flat schema: leagues, teams, coaches (`team_id UNIQUE`), players with `market_value NUMERIC`. |
| 000002 | Indexes. **Postgres doesn't index foreign keys automatically**, so `players(team_id)` and `teams(league_id)` are added; a **partial index** for free agents (`WHERE team_id IS NULL`); and `(name, id)` composite indexes that match the `ORDER BY`, so no sort is needed. |
| 000003 | **The temporal domain model, the most important migration.** It adds `seasons` (`CHECK end > start`), `team_seasons`, `transfers` (with type and fee checks), `player_market_values` (`UNIQUE(player_id, valued_on)`) and `coach_spells`. It drops `coaches.team_id UNIQUE` (a club has many coaches over time). It **converts money** from `NUMERIC` to `market_value_minor BIGINT` with `ROUND(x*100)`. It **backfills** an opening transfer, valuation and spell for existing rows, so no history is silently empty. Plus indexes for every history read. |
| 000004 | `team_editors (user_id, team_id)` primary key, *no FK* to users (another database), `ON DELETE CASCADE` from teams, and a reverse index. This replaced grants in the JWT. |
| 000005 | Constraint gaps: normalises bad `coach_spells.role` values, then adds the `CHECK`. **Season overlap is prevented with an exclusion constraint**: `EXCLUDE USING gist (daterange(start_date, end_date, '[]') WITH &&)` using `btree_gist`. Overlapping seasons would make "which season contains this date" ambiguous. |
| 000006 | **Resolving two contradictory rules.** `ON DELETE SET NULL` on both club FKs ("history survives a club deletion") plus `CHECK (from IS NOT NULL OR to IS NOT NULL)` meant deleting a club with an opening-transfer row failed. A CHECK can't tell an insert from a cascade, so the CHECK was dropped and the rule enforced in `validateTransfer` at creation, documented with `COMMENT ON TABLE`. A great interview story about constraint design. |
| 000007 | Search: `pg_trgm`, a **generated `tsvector` column** (`GENERATED ALWAYS AS ... STORED`, so it can't drift the way a trigger-maintained column can), weighted A/B/C/D, **GIN** indexes, and trigram GIN indexes on names. The `'simple'` configuration is used, because English stemming mangles proper nouns. |
| 000008 | Profile depth: `nationalities TEXT[]` replaces `nationality` + `second_nationality` (two columns can't express three), with a data migration; the search column is rebuilt to read the arrays; scouting percentages with a range `CHECK`; strengths and weaknesses arrays; `player_notes` with `UNIQUE(player_id, author_id)` (one note per person, enforced by the database, since two simultaneous posts would both pass a check in code) and `CHECK(length(btrim(body)) > 0)`. |
| 000009 | `tactic_boards`: a unique slug, `content JSONB` with `CHECK (jsonb_typeof(content) = 'object')`, and an owner-name index that matches the list ordering. |
| 000010 | `articles`: a status `CHECK`, **`CHECK (status <> 'published' OR publish_at IS NOT NULL)`** (an invalid published state is impossible to store), a **partial index** for live articles (`WHERE status = 'published'`), and a GIN index on tags. |
| 000011 | `player_season_stats` (non-negative check), international caps and goals, and **`external_refs (source, kind, external_id) → local_id`**, the importer's idempotency table. |

### 5.9 The Transfermarkt importer (`cmd/import-transfermarkt`, `internal/importer`)

- **`cmd/import-transfermarkt/main.go`**: flags `-data`, `-download`,
  `-leagues`, `-first-season`, `-dry-run`. The database comes from
  `DATABASE_URL` or `DB_*` **and is never a flag**, so a password never lands
  in shell history. `fetch` downloads each `.csv.gz` to a `.part` file and
  **renames** it on completion, so an interrupted download never leaves a
  truncated file that looks whole. It prints a report of what was created,
  matched, updated and could not be reconciled.
- **`dataset.go`**: streams the gzipped CSVs (clubs, players, transfers,
  valuations, games, appearances, ~65 MB) and keeps only the chosen scope.
- **`importer.go`**: the three rules in the package comment:
  1. **Idempotent**: every created row is recorded in `external_refs`, so a
     second run updates instead of duplicating.
  2. **Hand-entered judgement wins**: matched players keep notes, scouting
     percentages, strengths, secondary positions, squad number and
     nationalities; facts the source is authoritative on (club, birth date,
     height, contract, value) are corrected.
  3. **All or nothing**: one transaction; `-dry-run` does everything, prints
     the report, and rolls back.

  It sets `SET LOCAL TIME ZONE 'UTC'` and checks that migration 000011 exists
  (`to_regclass`) before doing anything, a direct lesson from production
  outages. The steps run in order: references → competitions → seasons →
  clubs → players → values → transfers → stats → entries → save references.
  **Bulk writes** pass Postgres arrays through `unnest($1::text[], ...)` in
  chunks of 5,000 (`inChunks`), so 100,000+ rows take seconds rather than
  hours of row-by-row inserts. Stats are replaced per player
  (`DELETE ... WHERE player_id = ANY(...)`, then insert).
- **`rules.go`**: the tested, pure logic.
  - `Catalog` maps Transfermarkt competition ids (`GB1`, `ES1`, `CL`, ...) to
    display names and types; `competitionsFor` brings in each league's cups
    and the European competitions.
  - **Name matching**: `fold` lower-cases, strips accents (NFD and drop
    combining marks) and maps `ø→o`, `ß→ss`, and so on. `clubKey` also drops
    legal-form tokens (`fc`, `ac`, `sv`, `vfb`, ...), so "Arsenal" matches
    "Arsenal FC". Players are matched by name **plus** date of birth.
  - **`Classify`** infers transfer types the source doesn't mark: a **zero-fee
    move reversed within two years (+60 days) is a loan, and the reversal is
    its return**; "Retired" becomes a retirement; "Without club" and similar a
    release; a move from a youth side (a regex for U19, II, B, Primavera,
    Juvenil, ...) with no fee a youth promotion; a zero fee a free transfer;
    otherwise permanent. A buy-back carries a fee, so it correctly stays two
    permanent moves.

**Why not import through the API?** The API is the right door for one editor
changing one player. For 6,600 players and 100,000 history rows it would mean
hours of requests, the per-account rate limit, and no way to make the load
atomic.

### 5.10 `internal/games/`

- **`games.go`**: the hub catalog (`slug`, `titleKey`, `descriptionKey`,
  `players`, `status`). Translation **keys**, not text, because the frontend
  owns the strings in each language.
- **`playedforboth/dataset.go`**: `//go:embed data/dataset.json` compiles the
  data into the binary. On the wire each record is a **positional array** (to
  keep the download small); custom `UnmarshalJSON` methods give each position
  a name for validation. `Load()` validates at boot.
- **`tools/played-for-both/extract-dataset.mjs`** (at the repository root): a
  one-off Node script that pulled the JSON out of a standalone reference page
  and refuses to write output that fails sanity checks.

**How I'd say it:** "The football service is a layered CRUD service with real
invariants. History tables are the source of truth, current state is derived
inside transactions with row locks and newest-wins guards, authorization is
centralised in one `Authorizer` with grants resolved per request, and the
database restates the important rules as CHECK, UNIQUE and exclusion
constraints as a last line of defence."

---

## Part 6 — identity-svc

Accounts, passwords, sessions, external sign-in, email verification and
captcha. Port 8080. **The only process holding the RSA private key.**

> Structural note to be ready for: identity-svc has **no separate service
> layer**. Handlers call repositories directly. Its only authorization rule is
> `requireAdmin`, and its logic is mostly credential protocol rather than
> domain rules, so the extra layer didn't earn its place. If asked, I'd say I'd
> add one as soon as it grew real business rules (organisations, invitations),
> to match football-svc.

### 6.1 `cmd/server/main.go`

- `auth.LoadSignerFromEnv()` (private key) → `db.ConnectFromEnv()` → the
  publisher → **five repositories**: users, refresh tokens, identities (OAuth
  links), login codes, email verification.
- `oauth.FromEnv(PUBLIC_BASE_URL)`: a provider without credentials is simply
  absent. `mailer.New` falls back to a log-only mailer without SMTP.
  `captcha.New` returns a disabled verifier without a secret. **Optional
  features degrade; they never stop the service.**
- `SecureCookies` is true only if the public URL is https (a Secure cookie is
  silently dropped on plain http, which would look like a state-check
  failure). `PublicPathPrefix` is derived from the same `PUBLIC_BASE_URL`
  (explained in 6.3).
- **Rate limits per IP**: login and resend-verification at 0.2/s with a burst
  of 5 (12 a minute sustained); register, refresh, exchange and verify at 1/s
  with a burst of 10. **Per account**: 10 failures, then 1 per 3 minutes.
- **Three janitors** (goroutines with a ticker, cancelled through a context,
  each run once at startup): expired refresh tokens hourly, OAuth login codes
  every 15 minutes, verification tokens hourly. The story: `DeleteExpired`
  existed, was on the interface and was stubbed in a test fake, but **nothing
  called it**, so the tables only grew.
- Routes: `/health`, `/.well-known/jwks.json` (public by design),
  `auth/{register,login,refresh,logout}`, `auth/providers`, `auth/config` (one
  call telling the frontend which providers exist, whether captcha is required
  and with which **public** site key, and whether verification is on),
  `auth/{provider}` and its callback, `auth/exchange`, `verify-email`,
  `resend-verification`, `users/me`, `users/me/password`,
  `users/me/identities`, and the admin routes `users`, `users/{id}/role`,
  `DELETE users/{id}`.

### 6.2 `internal/handler/auth.go`

- **`Register`**: the captcha is verified **before** bcrypt (hashing is the
  expensive part, and exactly the work a sign-up flood wants to cause). It
  validates the username, email and password, hashes with
  `bcrypt.DefaultCost`, and always creates `role = user`. **`RegisterRequest`
  has no Role field**: a client-supplied role would let anyone register as
  admin. If sending the verification email fails, registration still succeeds
  (the account exists; a resend can fix it). The response is a named type
  `RegisterResponse{user, verification_required, message}`, changed from a map
  literal after a shape change broke a test silently.
- **`Login`**:
  1. Captcha first.
  2. The **per-account limiter**, keyed by the lowercased identifier:
     `Blocked` is checked first, and only failures `Record`. Unknown
     identifiers are counted too, otherwise only real accounts would get
     blocked, which leaks which ones exist.
  3. **Timing-safe enumeration defence**: if the user doesn't exist, it still
     runs `bcrypt.CompareHashAndPassword` against a **dummy hash generated at
     startup with the same cost**. Without that, "no such user" returns
     ~60–100 ms faster than "wrong password", and the identical bodies don't
     matter. `timing_test.go` checks this.
  4. **Email verification is checked only after the password matches.**
     Reporting "unverified" earlier would reveal that the address is
     registered.
  5. `issueTokens` → `writeTokens`.
- **`Refresh`**: looks the token up by its **SHA-256 hash**. If it's not
  active and was revoked **more than 30 seconds ago** (`rotationGrace`), that
  is **reuse detection**: `RevokeAllForUser`. Within 30 seconds it's two tabs
  racing on one cookie, and only that request is refused. Otherwise it rotates
  (a new random token, stored hashed) and mints an access token **with the
  role re-read from the database**, so a role change takes effect within one
  access-token lifetime.
- **`Logout`**: always 204 (whether the token existed must not leak); clears
  the cookie first, so the browser lets go of it even if the revoke fails.
- **`UpdateRole`** (admin): reads the old role (the event carries the
  transition), updates, publishes `UserRoleChanged`, then **revokes all of the
  user's refresh tokens**. Otherwise a demoted admin would keep a valid
  session.
- `userAgent` truncates the stored User-Agent to 512 bytes (an
  attacker-controlled, unbounded header).

### 6.3 `session_cookie.go`: the refresh token in an HttpOnly cookie

- Opt-in per request with `X-Session-Cookie: 1`. Browsers send it; scripts and
  smoke tests keep receiving the token in the body.
- Cookie `sp_refresh`: **HttpOnly** (no script can read it), **SameSite=Strict**,
  `Secure` when the request came over https (`r.TLS` or
  `X-Forwarded-Proto`), `Path` scoped to the auth endpoints **as the browser
  sees them**.
- **CSRF** is covered twice: SameSite=Strict, **and** the cookie is only read
  when the custom header is present, which a cross-origin page can't add
  without a CORS preflight. Either alone would be enough.
- **The path bug**: the gateway mounts the service under `/api/identity` and
  strips the prefix, so a cookie scoped to `/api/v1/auth` was scoped to a URL
  the browser never visits and was never sent back: every reload signed the
  user out, and every OAuth callback failed as "expired" in under a
  millisecond. The fix is `cookiePath()`: a configured prefix (from
  `PUBLIC_BASE_URL`) first, then a validated `X-Forwarded-Prefix` (Caddy sends
  it; **Vercel rewrites don't**, which is why the configured value exists).
- A legacy migration path: a browser still holding a token from the old
  localStorage version can present it in the body once, to be moved onto the
  cookie.

### 6.4 `oauth.go` + `internal/oauth/provider.go`: Google and Facebook

- **`StartOAuth`**: generates a random `state` (CSRF defence for the callback:
  without it an attacker can make a victim complete a sign-in into the
  attacker's provider account) and a **PKCE** verifier, stores both in
  short-lived HttpOnly flow cookies (10 minutes, SameSite=Lax because the
  callback is a cross-site navigation), and redirects with
  `S256ChallengeOption`.
- **`CompleteOAuth`**: a browser navigation, so failures **redirect to the
  frontend with an error code** instead of returning JSON nobody would see.
  It checks the state cookie against the query, exchanges the code with the
  verifier, normalises the profile, and resolves the account. Then the key
  trick: **it doesn't put tokens in the redirect URL** (they'd end up in
  history, access logs and Referer headers). It issues a **single-use login
  code that expires in 60 seconds**, stored hashed, and redirects to
  `/auth/callback?code=...`.
- **`ExchangeCode`**: the frontend POSTs the code. `Redeem` is **one atomic
  statement**, `UPDATE ... SET used_at = NOW() WHERE code_hash=$1 AND used_at
  IS NULL AND expires_at > NOW() RETURNING user_id`, so two concurrent
  exchanges can't both succeed. Unknown, used and expired codes all return
  the same error.
- **`resolveAccount`: the account-linking policy.**
  1. Already linked by `(provider, provider_user_id)`: sign in. **Keyed on the
     provider's immutable subject id, never the email**, because emails can be
     changed or reassigned at the provider.
  2. A local account has this email: link **only if the provider says the
     email is verified**. Otherwise it's an **account-takeover route** (anyone
     who can register that address at the provider would claim the account).
     That returns a Conflict, shown as "sign in with your password and link
     from settings".
  3. Otherwise create an account: no password (the column became nullable in
     000003, rather than storing a placeholder that could later be compared
     against), `role = user`, a username de-duplicated from the display name,
     and a synthetic `@users.noreply.scoutpulse` address if Facebook withheld
     the email. `EmailVerified` follows the provider; Facebook never reports
     verification, so it's never auto-linked.
- `provider.go` normalises each provider into one `Profile` type, so a third
  provider is one entry there and nothing else.

### 6.5 The other identity files

| File | What |
|---|---|
| `users.go` | `ChangePassword`: OAuth-only accounts get a clear message instead of "invalid credentials"; the current password is checked; the new one must differ; then **all sessions are revoked**. `ListUsers` (admin, substring search). `DeleteUser`: admin, **can't delete yourself** (one mis-click could leave no admin), reads the username first (the event needs it and the row will be gone), deletes (sessions cascade), publishes `UserDeleted`. |
| `verification.go` | `sendVerificationEmail`: a hashed single-use token valid for 24 hours. The link points at the **frontend** page, which POSTs the token back, keeping it out of a state-changing GET. `VerifyEmail`, `ResendVerification` (captcha plus a strict limit, since it sends mail to an address the caller chooses), `AuthConfig`. |
| `captcha/captcha.go` | One verifier for **reCAPTCHA and Cloudflare Turnstile** (the same form fields and response shape), with an optional v3 `MinScore`. The security point: **the widget proves nothing; verification on the server is the control**, since a bot just calls the API directly. |
| `mailer/mailer.go` | An SMTP mailer, or a `LogMailer` without `SMTP_HOST` (the link is written to the log, so the flow is testable locally). `header()` strips CR/LF from header values to prevent **email header injection** (a newline in a subject could add a `Bcc:`). |
| `validate/validate.go` | Username: 3–32 runes, letters, digits and `.-_` only, no leading or trailing separator (`"admin "` vs `"admin"` impersonation). Email: `net/mail.ParseAddress`, rejecting the display-name form, max 254, **lower-cased** (so OAuth linking finds `A@x.com` = `a@x.com`). Password: at least 8, and **at most bcrypt's 72-byte limit**, so a long passphrase gives a clear 400 instead of an opaque 500. |
| `model/user.go` | `User` (`password_hash` is `json:"-"`), `Role` + `ValidRole`, `RefreshToken.Active(now)`, `Identity`. |
| `repository/*.go` | `postgres_user.go` (`GetByIdentifier` matches email or username), `refresh_token.go`, `identity.go` (links + login codes), `email_verification.go`. All tokens and codes are stored as **SHA-256 hashes**: unsalted is fine because they're 256 bits of randomness (not guessable, unlike passwords), and a slow KDF would only add latency. The hash means a database leak doesn't hand over live sessions. |

**`refresh_token.go` `Rotate`** deserves its own mention: one transaction
inserts the new token, then `UPDATE ... SET revoked_at = NOW(), replaced_by =
$new WHERE id = $old AND revoked_at IS NULL`. If **0 rows** are affected,
another request already rotated it, so this one is refused. **The `revoked_at
IS NULL` predicate is the concurrency control**: exactly one of two concurrent
refreshes wins, without an explicit lock.

### 6.6 Identity migrations

| # | What |
|---|---|
| 000001 | `users` (with the old `managed_team_ids UUID[]`). |
| 000002 | `refresh_tokens` (hash `BYTEA UNIQUE`, expiry, `revoked_at`, a `replaced_by` self-FK forming the rotation chain, user agent), **drops `managed_team_ids`**, adds `role_updated_at/by`, and a role `CHECK`. |
| 000003 | `password_hash` becomes nullable; `user_identities` with `UNIQUE(provider, provider_user_id)` and `UNIQUE(user_id, provider)`; `oauth_login_codes`. |
| 000004 | `email_verified` (existing users backfilled to true, so nobody is locked out), `email_verification_tokens`. |

**How I'd say it:** "Identity is about protocol correctness. Asymmetric JWTs so
only it can mint; a short access token in memory; a long refresh token that's
opaque, hashed at rest, rotated on every use, and kept in an HttpOnly
SameSite=Strict cookie; reuse detection that revokes every session, with a
30-second grace for two tabs racing; timing-safe login; OAuth with state, PKCE,
a one-time code instead of tokens in the URL, and linking only on a verified
email."

---

## Part 7 — Frontend foundation (Angular 21, SSR)

### 7.1 The concepts first

- **Standalone components only**: no NgModules. Each component declares its
  own `imports`.
- **Signals** for all state: `signal()` for writable state, `computed()` for
  derived values, `effect()` for side effects, `input()`/`output()` for
  component I/O. **Zoneless** change detection
  (`provideZonelessChangeDetection`): no zone.js; signals tell Angular exactly
  what changed. Every component is `ChangeDetectionStrategy.OnPush`.
- **The new control flow**: `@if`, `@for`, `@defer (on viewport)`.
- **`resource()`**: async data as a signal (`value()`, `isLoading()`,
  `error()`), re-run when its `params` signal changes.
- **SSR + hydration**: the server renders HTML per request; the browser
  *hydrates* (attaches to the existing DOM instead of re-rendering).
  `withEventReplay()` records clicks made before hydration finishes and
  replays them afterwards.
- **Dependency inversion through `InjectionToken`s**: components depend on
  interfaces, never on HTTP classes.

### 7.2 Configuration files

| File | What and why |
|---|---|
| `package.json` | Angular 21.2, `@angular/ssr`, Express 5, Tiptap 3.31 (pinned exactly, since it's an editor whose behaviour changes between versions), Three.js r169, `flag-icons`, rxjs. Dev: Vitest 4, jsdom, Prettier, TypeScript 5.9. `build:vercel` = `ng build && node scripts/vercel-postbuild.mjs`. Node 22. |
| `angular.json` | The `@angular/build:application` (esbuild) builder, `outputMode: "server"`, `ssr.entry: src/server.ts`. The flag SVGs are copied from `node_modules` as assets. **Budgets**: initial bundle warn at 420 KB / error at 550 KB; any component stylesheet warn at 4 KB / error at 8 KB. So the build *enforces* lazy loading and lean components. `ng test` uses the `unit-test` builder with Vitest. |
| `tsconfig*.json` | Strict TypeScript and strict templates. |
| `vitest.config.ts` | `pool: 'threads'`. The default `forks` pool timed out spawning workers on Windows (made worse by a virus scanner) and reported "no tests" rather than an error; threads also made the run ~7× faster. |
| `proxy.conf.json` | `ng serve` on :4200 forwards `/api` and `/.well-known` to the gateway on :8000, so dev uses the same same-origin layout as production. |
| `.prettierrc`, `.editorconfig`, `.nvmrc` | Formatting (checked in CI) and the Node version. |
| `vercel.json` | Build command, `outputDirectory: dist/frontend/browser` (static assets on the CDN), the `api/ssr.mjs` function with `includeFiles: dist/frontend/server/**`, and **rewrites**: `/api/football/*` and `/api/identity/*` → the Railway hosts, JWKS → identity, and everything else → `/api/ssr`. Vercel plays the gateway's role in production. |
| `Dockerfile` | Two stages. Manifests are copied first so `npm ci` is cached until dependencies change; the runtime has production dependencies only, runs as `USER node`, and uses an **exec-form** `CMD` (Node is PID 1 and receives SIGTERM). It sets `GATEWAY_INTERNAL_URL`, `SITE_URL` and `NG_ALLOWED_HOSTS`. |

### 7.3 Bootstrap and rendering files

- **`src/main.ts`**: `bootstrapApplication(App, appConfig)` in the browser.
- **`src/main.server.ts`**: the same with the merged server config, receiving
  a `BootstrapContext` per request.
- **`src/index.html`**: the default title and description, `color-scheme`,
  favicons, and **an inline script that applies the saved theme before the
  first paint**. Angular would run too late and cause a flash of the wrong
  theme. That inline script is one reason the CSP needs hashes.
- **`app/app.ts`** / **`app.html`**: the root renders `<app-shell>`.
- **`app/app.config.ts`**:
  - `provideRouter(routes, withComponentInputBinding(),
    withInMemoryScrolling(...))`: route params arrive as component `input()`s.
  - `provideHttpClient(withFetch(), withInterceptors([error, auth,
    correlationId]))`. `withFetch` matters for SSR: the fetch backend lets
    Angular **transfer HTTP responses** from the server render to the client,
    so hydration doesn't refetch everything.
  - `provideClientHydration(withEventReplay())`.
  - `API_CONFIG` = `apiConfigFor('')` (root-relative URLs) and `SITE_URL` =
    the document origin.
  - `...provideFootballApi()`: every repository binding.
  - `provideAppInitializer`: **restore the session before the first route
    renders**, browser only. On the server: `markAnonymous()` and return
    immediately (no cookie, no point blocking every render on a network call).
- **`app/app.config.server.ts`**: `provideServerRendering(withRoutes(...))`
  and **different values for the same tokens**. `API_CONFIG` must be
  **absolute** on the server (Node has no origin to resolve a relative URL
  against): `GATEWAY_INTERNAL_URL`, or a `FOOTBALL_API_URL` +
  `IDENTITY_API_URL` pair (both or neither, since half an override looks like
  one feature being broken). `SITE_URL` comes from the environment, **never
  from the Host header** (attacker-controlled; building canonical links from
  it would let anyone point this site's canonical tags elsewhere).
- **`app/app.routes.ts`**: every route uses **`loadComponent`** (lazy), so the
  initial bundle is the shell only. Literal paths come before parameters
  (`clubs/new` before `clubs/:id`). Guards: `authGuard`, `editorGuard`,
  `adminGuard`, plus a `canDeactivate` "leave editor?" check typed
  *structurally*, so this file doesn't import the editor and pull it out of
  its lazy chunk. `games` loads a child route tree (`GAMES_ROUTES`).
- **`app/app.routes.server.ts`**: **hybrid rendering per route.**
  - `RenderMode.Server` (per request) for the public SEO surface: transfers,
    players, clubs, competitions, coaches, seasons, search (rendered, but
    `noindex, follow`), articles, games, and a single tactics board (`noindex`,
    but server-rendered so a pasted link **unfurls** with its title).
  - `RenderMode.Client` for every form, login and register, the account and
    admin pages, the desk, and the tactics editor and list. Reasons: nothing
    to index, **guards can't be evaluated on the server** (no session), and
    they need a token Node doesn't hold.
  - **Not `Prerender`**: the generated default is build-time generation,
    impossible for `/players/:id` because the ids live in a database the build
    has never seen.
  - **Every public route is listed explicitly**, and the final `**` route has
    `status: 404`. Without that, the client router's own `**` "not found" page
    counts as a successful render, so **every mistyped URL answered 200 (a
    soft 404)**. A side effect is that forgetting to register a new public page
    makes it 404 loudly. CI asserts both directions.
- **`src/server.ts`**: the Express 5 SSR server.
  - `AngularNodeAppEngine({ allowedHosts, trustProxyHeaders })`.
    **`NG_ALLOWED_HOSTS`**: Angular 21 validates the Host header (an SSRF and
    cache-poisoning defence) and, **if it fails, silently falls back to client
    rendering**. The first deploy "did nothing" for exactly this reason.
    **`TRUST_PROXY_HEADERS`**: behind a proxy the real host is in
    `X-Forwarded-Host`; it's off by default because trusting it is only sound
    when something upstream sets it.
  - **A hand-built Content-Security-Policy**, because Angular's `autoCsp`
    refuses to run with SSR. `script-src 'self'` plus **SHA-256 hashes** of
    the template's inline scripts (computed once at startup from
    `index.server.html` / `index.csr.html`), plus, **per response**, the hash
    of Angular's event-replay bootstrap script **only if it matches an exact
    regex** (a list of lowercase event names and nothing else), so injected
    markup can't pass as it. `'unsafe-hashes'` with the hash of the
    stylesheet's `onload="this.media='all'"`, and the captcha origins.
    Also `object-src 'none'`, `base-uri 'self'`, `form-action 'self'`,
    `frame-ancestors 'none'`.
  - Other headers: `X-Frame-Options: DENY` (for browsers predating
    `frame-ancestors`), `nosniff`, `Referrer-Policy`, `Permissions-Policy`
    (camera, microphone, geolocation, payment and USB off).
  - `express.static` with `maxAge: '1y'` (hashed filenames), then the Angular
    handler; HTML responses get their per-page CSP header.
- **`api/ssr.mjs`: the Vercel adapter** (the hardest deployment problem):
  - Vercel's Angular preset publishes a *static* site, and `outputMode:
    server` emits no `index.html`, so the first deploy **built successfully
    and returned 404 for every URL**.
  - This function loads the server bundle with a **runtime dynamic `import()`
    of an opaque path**. A static import would let esbuild inline the bundle
    into the function, and the lazy route chunks loaded with `import()` don't
    survive that. It tries two candidate paths, because where the compiler
    puts the function file determines which one is right.
  - `applyRuntimeConfig()` runs **before** the import (the bundle reads env at
    module scope): `NG_ALLOWED_HOSTS` = **the union** of Vercel's own hostnames
    (`VERCEL_URL`, branch URL, production URL) and anything configured. A
    union rather than a replacement, because it was once set to the gateway's
    hostname and every page answered 400. `SITE_URL` must be absolute (a bare
    hostname is rejected with a warning). `GATEWAY_INTERNAL_URL` defaults to
    the deployment's own public origin, so SSR fetches go through the **same
    rewrites** and the Railway URLs live only in `vercel.json`.
    `TRUST_PROXY_HEADERS` is always true here.
  - The renderer is cached in a module variable, so a warm invocation doesn't
    re-evaluate the ~800 KB bundle.
- **`scripts/vercel-postbuild.mjs`**: deletes `browser/index.csr.html`.
  Vercel serves filesystem matches before rewrites and mapped that file to
  `/`, so **only the home page** was a bare client shell while every other
  route was server-rendered.

### 7.4 `core/`: singletons (depends on nothing in `features` or `shared`)

**`core/tokens/`**

- `api-config.ts`: the `API_CONFIG` token, with `BROWSER_GATEWAY = ''`. It used
  to be `http://localhost:8000`, frozen into the bundle at build time, so the
  deployed site asked **the visitor's own machine** for data
  (`ERR_CONNECTION_REFUSED`). Root-relative also removes CORS and cross-site
  cookies from the picture entirely. `DEV_GATEWAY`, `apiConfigFor` and
  `apiConfigOf` support the gateway-less dev mode (`scripts/dev-run.ps1`).
- `site-url.ts`: the `SITE_URL` token and `normaliseOrigin`.

**`core/models/`**: `football.ts`, `identity.ts`, `tactics.ts`,
`articles.ts`, `games.ts`: TypeScript mirrors of the API's wire types
(`MinorUnits = number`, nullable fields typed as `| null`).

**`core/api/`**

| File | What |
|---|---|
| `page.ts` | `Page<T>` mirrors the envelope; `emptyPage`, `nextPage` and `previousPage` (offset paging, with its trade-off documented: rows can skip or repeat if data changes between pages). |
| `api-error.ts` | `ApiError(code, message, status, requestId)`, `fromHttp` (status 0 → `'network'`, so a CORS or offline failure isn't reported as a server error), `isRetryable`, `isUserFixable`. |
| `interceptors.ts` | `errorInterceptor` (everything becomes an `ApiError`) and `correlationIdInterceptor` (`crypto.randomUUID`, with a fallback on non-secure contexts). |
| `contracts.ts` | **Reader/writer interfaces per resource** (`PlayerReader`, `PlayerWriter`, ...) plus an `InjectionToken` each. This is **interface segregation**: the public player page injects only `PLAYER_READER`, so **the compiler** guarantees it can't call a write. |
| `providers.ts` | `provideFootballApi()`: the **only** file naming concrete classes. Reader and writer tokens both resolve to the same instance via `useExisting`. |
| `http/http-repository.ts` | An abstract base: `url(...segments)` (each segment `encodeURIComponent`-ed), `params()` drops undefined/empty values (`?team_id=` isn't the same request as no filter), `getPage`/`getOne`/`post`/`put`/`deleteAt`, all returning Promises via `firstValueFrom`. Adding a resource is a subclass plus a binding (**open/closed**). Errors are not caught here; the interceptor has already typed them. |
| `http/football-repositories.ts`, `identity-repositories.ts`, `tactic-repository.ts`, `article-repository.ts`, `game-repository.ts` | The concrete repositories. The game repository caches the dataset at root level so it outlives the page. |
| `lookup-store.ts` | Resolves club, competition and season ids to names. The API returns ids, not embedded objects, so instead of an N+1 (25 rows, 50 requests) each list is **loaded once and cached**, paging through `limit=100` (the API clamps, so a single big request would silently truncate), with an in-flight promise so ten callers make one request, and a 5,000-row safety stop. **Its limit is documented**: past a few thousand clubs it needs `?ids=` or `?expand=`. |
| `player-names.ts` | Players are too many to load all, so this resolves **only the ids on screen** with one `?ids=a,b,c` request and caches them. That's what removed the transfer feed's N+1 (which also ran during the server render of the landing page). |
| `transferred-map.ts` | Carries a lookup `Map` from the server render to the browser through **`TransferState`** (as an array of pairs, since a `Map` doesn't survive JSON). Read once and removed. Without it, the server renders real names, the browser's first render has an empty map and renders dashes, and that's a **hydration mismatch**. |
| `ssr-resource.ts` | **`ssrResource(name, {params, loader})`**: Angular 21 transfers HTTP responses but **not `resource()` state**, so the browser started with `value()` undefined and rendered a skeleton where the server had rendered a table. Angular threw away the server DOM, the page collapsed and grew back. The wrapper stores the server's **first** resolved value in `TransferState` and passes it as `defaultValue` in the browser. Keyed by name only: keying by params would mean evaluating `params()` during field initialisation, which reads required inputs before Angular sets them and throws. That broke the browser on every navigation and was invisible on the server. The template must branch on `value()` **before** `isLoading()`. |
| `articles.ts`, `tactics.ts`, `games.ts`, `user-admin.ts` | Contracts and tokens for those areas. |

**`core/auth/`**

| File | What |
|---|---|
| `token-storage.ts` | **No credential is stored.** Only a `signed_in` flag in localStorage, meaning "a session cookie probably exists, so a restore is worth one request" (script can't see an HttpOnly cookie, and calling refresh for every anonymous visitor would be wasteful). It reads a legacy refresh token once, to migrate it. Every method is a no-op on the server, and storage access is wrapped in try/catch (private browsing can throw). |
| `session-store.ts` | Three private signals: `accessToken` (**memory only**), `user`, `status: 'unknown' \| 'anonymous' \| 'authenticated'`. `'unknown'` exists so a reload doesn't flash the signed-out UI before the restore finishes. `isAdmin`, `isEditor` and `role` are **computed**, so they can never disagree with the user. `claims()` decodes the JWT payload for display only ("never a security boundary"). |
| `auth-repository.ts` | The identity calls. They set the `SKIP_AUTH` `HttpContextToken` (so the auth interceptor never retries login or refresh, which could loop forever) and send `X-Session-Cookie: 1` (same-origin requests carry the cookie automatically, so no `withCredentials` is needed). `isUnauthorized()` helper. |
| `auth-interceptor.ts` | **The single-flight refresh.** `let inFlightRefresh: Promise<TokenPair> \| null` lives at **module scope**, because each request runs a new invocation of the interceptor function, so anything inside it can't be shared. The first 401 starts the refresh (`??=`); every other 401 awaits **the same promise**; all retry with the new token; `finally` clears the slot so a *later* 401 can refresh again. A refused refresh clears the session; a network failure keeps it. **Why it matters:** refresh tokens are single use, and reuse revokes every session. A page firing four requests with an expired token would otherwise trigger four refreshes; three would present a spent token, and the user would be **logged out everywhere for loading a page**. |
| `auth-facade.ts` | Orchestration, so components never sequence auth calls: `login` (tokens, then profile), `register` (signs in only when verification isn't required), `completeOAuth(code)`, `logout` (clears locally **whatever the server says**), `restore()` (an unauthorized response clears the session; a network error or cold start keeps it for the next load). |
| `guards.ts` | `authGuard`, `editorGuard`, `adminGuard`. **Not security**, only navigation, documented as such. A signed-in user without permission goes home, not to login (logging in again wouldn't help). `returnTo` carries the attempted URL. |
| `permissions.ts` | Mirrors the server's `Authorizer` so the UI only offers actions that will succeed (an Edit button that always 403s teaches people to distrust the interface). An `effect` loads grants from `GET /users/{id}/teams` when a user signs in and drops them on sign-out. `ready` prevents a form flashing. Methods: `canEditTeam`, `canMoveBetween(from, to)` (either club, the transfer rule), `canEditPlayer`, `canAdminister`. |

**`core/seo/seo.ts`**: `describe({title, description, path, type, noindex,
image})` sets the title, meta description, canonical link, `og:*`,
`twitter:card`, `robots: noindex, follow` when requested, and JSON-LD
structured data (managing the `<script type="application/ld+json">` element
directly). It uses `updateTag`, not `addTag`, so navigating between players
doesn't accumulate tags. **This is the payoff for SSR**: the tags are in the
first HTML response, which link unfurlers and crawlers need.

**`core/theme/theme.ts`**: `ThemeStore` with three states (`system`, `light`,
`dark`), because a boolean can't express "follow my OS". It only stamps
`data-theme` on `<html>`; the palette is pure CSS (`styles.css` defines it on
`:root`, under `prefers-color-scheme: dark`, and under `[data-theme='dark']`).

### 7.5 `shared/` and `layout/`

- **`shared/ui/`**: `states.ts` (`app-loading` skeleton with an `aria-live`
  status, `app-empty`, `app-error-state` that shows the **request id** so a
  user can quote it), `confirm-dialog.ts`, `actions.ts`.
- **`shared/forms/`**: `field.ts` (label, hint and error wrapper),
  `submit.ts`, `club-select.ts`, `captcha.ts` (renders Turnstile or reCAPTCHA
  from `auth/config`), `provider-buttons.ts` (only the providers that exist).
- **`shared/pagination/paginator.ts`**: previous/next plus "showing 26–50",
  because the API reports `has_more` rather than a total (a `COUNT` on every
  request is expensive).
- **`shared/pipes/money-pipe.ts`**: the **single** place minor units become a
  decimal. `Intl.NumberFormat`, a compact mode (`€85.0m`, `€125m`; one decimal
  below 100), and null → "—" (undisclosed, which is different from free = 0).
- **`shared/util/`**:
  - `dates.ts` converts between `<input type="date">` (`1998-03-05`) and the
    API's RFC 3339 form (`1998-03-05T00:00:00Z`) **by string slicing, never
    `new Date()`**. A `Date` parses the date as UTC midnight and reads it back
    in local time, so anywhere west of Greenwich a birthday silently becomes
    the day before.
  - `money-input.ts` `parseMoney("25.50")` → `2550` by parsing the digits on
    each side of the point **as integers**. The naive
    `Math.round(parseFloat(x) * 100)` is subtly wrong (`25.55 * 100` is
    `2554.9999999999995`). An empty field is `null` (undisclosed), not 0.
- **`layout/shell.ts`**: the masthead (wordmark, Beta badge kept outside the
  link so its accessible name stays "ScoutPulse"), search, navigation, theme
  toggle, account menu, footer, and a **skip-to-content** link. No standings
  or fixtures in the nav: there's no match data, and a link to an empty page is
  worse than none.
- **`styles.css`**: design tokens (`--ground`, `--surface`, `--ink`,
  `--accent`, `--space-*`, `--text-*`), three theme blocks, base typography.
  Vanilla CSS, no framework.

**The dependency rule** (stated in `docs/FRONTEND.md`): `features → shared →
core`, never the reverse, and **features never import each other**.

**How I'd say it:** "The frontend is Angular 21, standalone, zoneless,
signals throughout, every route lazy. Rendering is hybrid per route: public
pages are server-rendered for SEO, anything needing a session is client-only.
The two hard SSR problems were state transfer (I wrote `ssrResource` and
`transferredMap` so the browser's first render matches the server's HTML) and
deployment (Host-header validation silently disabling SSR, and adapting
Angular's Node server to a Vercel function). Data access goes through
reader/writer interfaces bound to tokens, and the auth interceptor shares one
refresh across concurrent 401s, because the backend's reuse detection would
otherwise log people out."

---

## Part 8 — Frontend features

Each folder under `features/` is one lazy-loaded area. They import from
`core` and `shared`, never from each other.

### 8.1 The database pages

| Folder | Files and what is interesting |
|---|---|
| `transfers/` | `transfer-feed.ts`: **the centre of the product** and the home page. Transfer-type filter tabs (the API also supports player, club, season and minimum-fee filters), the paginator, club names through `LookupStore` and player names through `PlayerNames` (one `?ids=` request per page). `transfer-form.ts`: **the only way a player changes club**. The interesting decision is what it *doesn't* ask: the API requires `from_team_id` to equal the current club exactly, so a selling-club dropdown would have one correct answer and every other choice would fail. The origin is shown as a fact and sent from the loaded player; the form asks only where the player is going. `TransferEdit` corrects type, fee and season only. |
| `players/` | `player-page.ts`: the profile, with **four `ssrResource`s** (player, transfers, stats, values) so the server-rendered page hydrates cleanly. It shows the career (transfers), a Performance table (season stats), the valuation chart, scouting strengths and weaknesses, and member notes; SEO uses `type: 'profile'`. `value-chart.ts`: a **hand-drawn SVG** sparkline (area + line + emphasised endpoint, an `aria-label` summary). No chart library, because one sparkline doesn't justify the dependency, and SVG renders identically on the server. `player-form.ts`: create/edit with list editors for nationalities, secondary positions, strengths and weaknesses; no club field on edit. `value-form.ts` (admin). `player-notes.ts`: everyone's notes plus "my note" (write, edit, delete), with a character counter mirroring the server's cap. |
| `clubs/` | `club-list.ts`, `club-page.ts` (the squad grouped by position order back to front, the coach, competition history, transfers in and out), `club-form.ts`, `season-entry-form.ts`. |
| `competitions/` | `competition-pages.ts`: list and detail, with a **"now vs a chosen season"** picker. "Now" reads `teams.league_id` (overwritten by relegation); a season reads `team_seasons` (history). Both are correct answers to different questions. `competition-form.ts`. |
| `coaches/` | `coach-page.ts` (career spells), `coach-form.ts`, `spell-form.ts`. |
| `seasons/` | `season-pages.ts` (list and forms). |
| `search/` | `search-page.ts`: ranked results with tabs to narrow by kind (`?kind=`), `noindex, follow`. |
| `auth/` | `login.ts` (captcha and provider buttons from `auth/config`, honours `returnTo`), `register.ts` (shows "check your email" when verification is required), `oauth-callback.ts` (reads `?code=`, calls `completeOAuth`, maps error codes like `email_taken` to helpful messages), `verify-email.ts` (POSTs the token; a GET with a side effect would be wrong). |
| `account/` | `account-page.ts`: profile, password change, linked providers. The password form is always shown, because `GET /users/me` doesn't report whether a password exists. I logged a `has_password` flag in `ISSUES.md` rather than guessing. |
| `admin/` | `user-list.ts` (search, change role, delete), `club-editors.ts` (grant and revoke). Names are resolved client-side, because identity-svc has no get-user-by-id endpoint. |
| `errors/` | `not-found.ts`: the page served with the real 404. |

### 8.2 The tactics board (`features/tactics/`)

**The idea:** a coach places players and equipment, draws arrows and zones,
and builds a drill **step by step**. Players watch it **animate** in 3D or 2D
through a link that needs no account, or as an exported video.

**`board/`: the engine, framework-free.**

| File | What |
|---|---|
| `model.ts` | The **document**. A board is **data, not video**: objects (players with team, number, keeper flag, bib; 12 kinds of equipment), steps (`duration`, `hold`, `note`, `pos` per object, optional `bend` control points, `air` for lofted passes, `highlight`), and drawings (arrow, line, pen, rect, ellipse, polygon, text; solid/dashed/wavy following coaching notation: run, pass, dribble). **Coordinates are metres** on a 105 × 68 pitch with the origin at the centre spot, so it looks the same on a phone and a projector, and a 20 m grid really is 20 m. **An object missing from a step stays where it was**, so adding a step copies nothing. Kits (5 patterns), pitch layouts (full, half, third, box, drill), `formation()` presets. **Versioned**: `normalizeDoc` is the single place an older shape is upgraded. |
| `geometry.ts` | Pure plane maths with no Three.js: easing (`easeInOut`, `easeOut`), `lerp2`, quadratic Bézier `quad`, distance to a polyline, point-in-polygon, dashes, curve sampling. Unit-tested. |
| `playback.ts` | **Pure functions: document + time → frame.** `timeline(doc)` builds hold and move segments. `frameAt(doc, timeline, t)` returns every object's placement: ease-in-out for players, ease-out for the ball (it decelerates), along a **quadratic curve** if bent, and for a lofted pass a **parabola** `h = 4·peak·p·(1−p)` with the peak proportional to distance and capped at 14 m. It also computes the heading (for facing), fades in objects that first appear, fades the previous step's arrows out during the move, and fades the drawings in during a hold. Because it's pure, **scrubbing, playing and recording draw the same frame for the same moment**, and it's testable without WebGL. |
| `player.ts` | `BoardPlayer`: the clock as signals (`time`, `playing`, `speed`, `loop`) with computed `timeline`, `frame`, `step`, `atEnd`, `finished`. `tick(dt)` is called **by the engine's `requestAnimationFrame`**, so the clock advances exactly once per rendered frame: the picture never runs ahead of the time shown. `recordVideo(canvas, player)`: `canvas.captureStream(30)` + **`MediaRecorder`**, choosing MP4 (avc1) if supported, else WebM (VP9), at 6 Mbit/s, with a short lead-in and tail. **Entirely in the browser; nothing is uploaded.** |
| `engine.ts` | `BoardEngine`, about 1,100 lines of **Three.js** with **no Angular in it**. It owns the `WebGLRenderer`, scene and lights, a `PerspectiveCamera` with `OrbitControls` and four presets (top, tactical, broadcast, behind) with eased camera tweens, plus an **`OrthographicCamera` for 2D mode** (straight down, pan and zoom without rotating). `setDoc` rebuilds the world; `setFrame` moves pieces and rolls the ball by the distance travelled. **Picking**: a `Raycaster` against the ground plane converts pointer → pitch metres, and hit-tests objects, drawings and handles (bend handles, drawing control points). It reports **gestures** through `EngineHandlers` (`select`, `moveObject`, `bend`, `editDrawing`, `place`, `stroke`) and **does not decide what they mean**. That's the editor store's job, so the read-only viewer uses the same engine unchanged. `dispose()` frees geometries, materials and textures (WebGL leaks memory otherwise). |
| `pieces.ts` | Meshes for players (kit pattern, number label, keeper colour, bib) and equipment. Sizes are **exaggerated on purpose** (a real player is a speck at pitch scale); equipment is scaled up less. Each piece is a `Group` tagged with its object id, which is how a click on any part is traced back. |
| `strokes.ts` | Arrows, lines, zones, text **and the pitch markings**, as real geometry (ribbons), so lines stay sharp when zoomed. |
| `textures.ts` | Grass, kits, ball and nets **painted on a canvas at runtime**: nothing is downloaded, it works offline once loaded, and kit colours are exact. |
| `board-view.ts` | The Angular host component. **Three.js is loaded with a dynamic `import('./engine')` inside `afterNextRender`**, so it never runs on the server (no WebGL there) and only pages showing a board download it. The fallbacks are "Setting out the pitch…" and a no-WebGL message. Inputs are pushed into the engine through `effect`s; it's destroyed through `DestroyRef`. |
| `board.spec.ts` | Geometry and playback tests. |

**`editor/`**

- `editor-store.ts`: `EditorStore`, signals for the document, step index,
  selection, tool, pen and preview. **The document is immutable**: every
  change builds a new object sharing what it didn't touch, so **undo/redo is a
  stack of references** (capped at 100), and the engine can detect changes
  cheaply by reference equality. **A drag is one undo entry**: the snapshot is
  taken when the gesture opens. Operations: add, move, bend, steps
  (add, duplicate, reorder, delete), drawings, kits, formations.
- `tactics-editor.ts/.html/.css`: the page. Save/load through
  `TACTIC_WRITER`; a `canLeave()` route guard plus `beforeunload` for unsaved
  work; keyboard shortcuts through a `document:keydown` host listener.
  **A signed-out visitor can build a board**: on "save" their draft goes to
  `sessionStorage` (`scoutpulse:tactics-draft`), they sign in, and
  `restoreDraft()` brings it back, marked dirty, ready to save.
- `inspector.ts` (the right panel: the selection's properties, or pitch, kits
  and formations) and `tool-palette.ts` (the left panel: one click per tool,
  players per team with auto-numbering).

**`viewer/tactics-viewer.ts`**: public and account-free; server-rendered for
link unfurling, `noindex`. **`list/tactics-list.ts`**: "my boards".
**`ui/`**: `playback-bar.ts` (the same bar in editor and viewer, so a coach
previews exactly what players will see), `color-pick.ts`, `icon.ts`,
`stage.css`.

### 8.3 The editorial platform (`features/articles/`)

| Folder | What |
|---|---|
| `doc/article-doc.ts` | The shared document types and the **client-side copies of the server's URL rules** (`safeLink`, `safeImage`, `parseVideo`, `parseTacticSlug`), so the editor, the reader and `article_body.go` agree on what is safe. |
| `doc/article-body.ts` | **The renderer: a recursive walk over the JSON tree in Angular templates**, one case per known node type; unknown nodes are skipped. **No HTML is ever built from the document** (no `innerHTML`), so pasted text is always text, and only addresses that pass `safeLink`/`safeImage` reach an attribute. A subtle detail: whitespace next to text in inline templates collapses to a space, so closing tags hug their content (a spec checks there are no stray spaces around bold words). The same component renders the editor preview, so a writer previews exactly what a reader gets. |
| `doc/tactic-embed.ts` | A playable board inside an article, mounted with **`@defer (on viewport)`**: none of Three.js is fetched until the reader scrolls to it, and the server never renders it. |
| `doc/video-embed.ts` | A **poster image until clicked**, then the iframe from **YouTube's no-cookie host** (or Vimeo). A YouTube embed is about a megabyte of script plus cookies, for a reader who may never press play. |
| `editor/tiptap-setup.ts` | **Tiptap 3 (ProseMirror)**, loaded with a **dynamic import** from the editor page only, so no reader downloads it and it never runs on the server. The schema is the starter kit **minus code blocks** plus three custom nodes (figure, tacticBoard, video) with node views. **Pasted HTML is parsed into the schema**, so formatting from another site comes across as headings, lists and links, and anything else is dropped. |
| `editor/article-editor.ts` | The page: **autosave 1.5 s after typing pauses** (debounced `setTimeout`), title and subtitle, cover, `canLeave()`. |
| `editor/article-toolbar.ts`, `article-canvas.ts`, `insert-dialog.ts`, `publish-panel.ts` | Formatting toolbar; the canvas; **one `<dialog>`** for link, image, cover, board and video (native focus trap and Escape), validating with the same rules; the side panel showing workflow state and **only the actions the API will accept from this person** (an editor submits; an admin publishes, approves, schedules or sends back), in plain words. |
| `desk/article-desk.*` | The desk list. An editor sees their own; an admin sees everyone's, with the **review queue** at the top. |
| `reader/article-page.*` | The public article: server-rendered, `type: 'article'`, cover as `og:image`. |
| `list/article-list.ts`, `ui/article-card.ts`, `ui/article-status.ts`, `ui/article-icon.ts` | The section front with tag filtering, cards, status badges, inline SVG icons. |

### 8.4 The trivia game "Who Played For Both?" (`features/games/`)

- **Rules**: two players share one screen with a host pressing the buttons. A
  pair of teams appears (two clubs, or a club and a national team); whoever
  first names a player who played for both gets the point, then the answers
  are revealed. A score, rounds, and an optional 20/30/60-second timer.
- **`played-for-both.engine.ts`**: **pure functions with an injected RNG**
  (`Rng = () => number`), so specs can draw thousands of pairs
  deterministically. Clubs and nations share one numeric **entity** space
  (clubs `0..C−1`, nations after). A pair is encoded as one integer
  `min × 100000 + max`, so the pool and the "recent pairs" window are plain
  number arrays and sets. `buildIndex` builds `byEntity` (entity → players)
  once. `buildPairPool` keeps only pairs with at least one matching player,
  filtered by the chosen leagues and options (famous only, allow nations).
  `drawPair` avoids the last 60 pairs (up to 50 tries, then accepts a repeat).
  A nation is the second side 35% of the time in random mode.
- **`played-for-both.store.ts`**: a thin layer of **signals** over the engine,
  expensive work memoised in `computed`, settings saved in localStorage.
- **`played-for-both.page.ts`** + `components/` (`league-picker`,
  `scoreboard`, `answer-list`, `team-emblem` drawing club colours).
- **`games-i18n.ts`** + `lang-toggle.ts`: **EN/TR/DE**, deliberately local
  (one dictionary per language and a signal), because the rest of the site is
  English only. Documented so it doesn't grow into a half-built platform i18n.
- **`hub/games-hub.component.ts`**: cards from `GET /games` (translation keys
  from the server, text from the client).
- **Backend side** (5.10): an embedded, positional-array JSON dataset, served
  gzipped with a strong ETag and a day of caching. **It's never fetched during
  a server render**; the page's title and rules are server-rendered for link
  previews.

**How I'd say it:** "The tactics board is the piece I'm proudest of
architecturally. The document is data in pitch metres; playback is a pure
function from document and time to a frame; the Three.js engine knows nothing
about Angular and only reports gestures; the editor store gives meaning to
gestures with an immutable document, so undo is a stack of references; and
Three.js is dynamically imported after first render, so it costs nothing on
the server or on pages without a board."

---

## Part 9 — Testing, file by file

**Philosophy:** test hardest what **fails silently**: the refresh race,
timing leaks, SSR falling back to client rendering, soft 404s, migrations that
can't be rolled back, and driver detail leaking into responses. Unit tests use
**fakes behind the repository interfaces** (that's what the interfaces buy);
integration tests use **real Postgres in Docker** via testcontainers.

### Go

| Test file | What it pins |
|---|---|
| `libs/auth/jwt_test.go` | Round trip; **a verifier cannot mint** (`TestVerifierCannotMint`); rejects garbage, empty, **a token signed by a different key (a forged admin token)**, and **fails closed with no verification key**; JWKS round trip; **key rotation**. |
| `libs/auth/hardening_test.go` | Weak (<2048-bit) keys rejected, including from JWKS; JWKS must be https off-cluster; refetch on unknown `kid`; refetch rate-limited; middleware errors are JSON; the scheme is case-insensitive. |
| `libs/auth/keyformat_test.go` | Quoted and `\n`-escaped PEM accepted; damaged keys still rejected. |
| `libs/db/db_test.go` | **The DSN escapes awkward passwords**; a zero `Config` still gets a bounded pool. |
| `libs/platform/httpx/httpx_test.go` | Status mapping; `WriteError` **never leaks the cause**; unknown errors stay generic; wrapped errors keep their kind; `DecodeJSON` rejects unknown fields and trailing objects. |
| `libs/platform/server/middleware_test.go` | Request id kept or generated; recover returns JSON 500; CORS (including **unconfigured sends no allow-origin**); chain order; rate limit blocks after the burst and is per client. |
| `libs/platform/server/logging_test.go` | Successful probes skipped; failing probes logged; the skip is an exact path match. |
| `libs/platform/observability/metrics_test.go` | The `/metrics` guard (token, Railway 404, open locally). |
| `football-svc/domain/money_test.go`, `page_test.go` | No float drift; JSON is an integer; decimals rejected; paging clamps; the sentinel row is trimmed. |
| `football-svc/repository/errors_test.go` | **A malformed UUID is Invalid, not Internal**; check violation → Invalid; driver detail never leaks. |
| `football-svc/repository/search_test.go` | `ToTSQuery` builds prefix terms, **strips query operators**, bounds term length, keeps non-ASCII letters. |
| `football-svc/service/authorizer_test.go` | Grant lookups are cached; **revocation is immediate**; invalidation is scoped to the user; either-team logic; nil target is admin-only. |
| `football-svc/service/football_test.go`, `league_test.go`, `player_profile_test.go` | RBAC per service (unauthenticated, user, wrong editor, right editor, admin); **an update cannot move a club or set a value**; validation tables. |
| `football-svc/service/transfer_test.go`, `transfer_update_test.go` | Either club may record; an unrelated editor is refused; **origin must match**; season attached when found and optional when not; delete is admin-only; **an update cannot move the player**. |
| `football-svc/service/tactic_service_test.go`, `article_service_test.go` | Tactics ownership, cap and slug retry; the full **article workflow** with an injected clock (scheduling goes live by the clock, bounds, withdraw, the desk shows editors only their own). |
| `football-svc/service/identity_events_test.go` | User deleted removes every grant and clears the cache; malformed events are dropped; an empty id is ignored. |
| `football-svc/service/testsupport_test.go` | The in-memory fakes, such as `stubEditors` (which counts store calls, so the cache test can assert on them). |
| `football-svc/handler/handler_test.go`, `ids_test.go`, `game_handler_test.go` | Service errors map to statuses; **internal errors don't leak**; **the path id wins over the body**; protected routes need a token; paging and filters; `?ids=` parsing; dataset ETag/304/gzip. |
| `football-svc/importer/rules_test.go` | Name folding; legal-form stripping; cups only with their league; **loan detection, a free move without a return stays free, a reversal outside the window isn't a loan**; the source's placeholders. |
| `football-svc/games/playedforboth/dataset_test.go` | Counts; known careers; flag codes. |
| `football-svc/internal/tests/*_integration_test.go` | **testcontainers-go**: start Postgres, apply the real migrations, exercise the repositories and flows (including articles). Skipped under `-short`. |
| `identity-svc/handler/auth_test.go` | Register **ignores a client-supplied role**; login; **refresh rotates**; **reuse revokes every session**. |
| `identity-svc/handler/timing_test.go` | **An unknown account still hashes**; the same response for unknown and wrong password; the dummy hash has the real cost. |
| `identity-svc/handler/account_limit_test.go` | The account limit **follows the account, not the address**, and counts only failures. |
| `identity-svc/handler/session_cookie_test.go` | The refresh token stays out of the body; refresh and logout use the cookie; ignored without the header; legacy migration; **two concurrent tabs don't revoke everything**; path and Secure flag; a round trip **through a mounted proxy**. |
| `identity-svc/handler/oauth_test.go`, `cookiepath_prefix_test.go` | **Links only when the provider verified the email**; keyed on provider id, not email; creates passwordless users; username sanitising; the cookie path matches the browser's URL. |
| `identity-svc/handler/users_test.go` | Password change **ends every session**; requires the current password; admin-only listing and delete; **can't delete yourself**. |
| `identity-svc/handler/integration_test.go` | Full auth flow against a real Postgres (testcontainers). |
| `identity-svc/validate/validate_test.go` | Email, username and password tables. |
| `tests/integration/stack_test.go` | **Black-box tests against the whole compose stack** (testcontainers' compose module): migrations applied; deleting a team turns its players into free agents; transfer history recorded; **a backdated transfer doesn't rewrite the current club**; **football-svc cannot mint tokens**. Its own module, so it can only see wire types, not internals. |

### Frontend (Vitest + jsdom)

| Spec | What it pins |
|---|---|
| `core/auth/auth-interceptor.spec.ts` (13) | **The single-flight refresh**: "issues ONE refresh for three simultaneous 401s"; requests retry with the renewed token; a later 401 can refresh again; the refresh uses the cookie, never a token from script; "never refreshes a failing refresh, which would loop forever"; a refused refresh clears the session, other failures keep it; recovers when another tab rotated the cookie first; migrates a legacy stored token; non-401 failures pass through. |
| `core/auth/permissions.spec.ts` (12) | Admin, editor and member rules mirror the server's. |
| `core/api/player-names.spec.ts` (8) | Batching, caching, de-duplication of in-flight requests. |
| `core/api/ssr-resource.spec.ts` | Seeded components construct and render; detail pages work with their route input set (the regression where `params()` read inputs too early). |
| `shared/pipes/money-pipe.spec.ts` (15), `shared/util/money-input.spec.ts` (16), `dates.spec.ts` (9) | Formatting, compact forms, undisclosed vs free; **parsing without floats**; date round trips without time-zone drift. |
| `features/tactics/board/board.spec.ts` (13), `editor/editor-store.spec.ts` (9) | Positions carried across steps, playback frames, geometry; **a whole drag is one undo step**; moving an object changes only the current step onwards; deleting, duplicating and reordering steps; auto-numbering; dirty/clean state. |
| `features/articles/doc/article-doc.spec.ts` (7), `article-body.spec.ts` (5) | URL safety rules; the renderer skips unknown nodes; **no stray spaces around inline marks**. |
| `features/games/**/*.spec.ts` | The engine with a seeded RNG (17 cases), the page, the i18n. |
| `app.spec.ts` | The app boots. |

### CI as tests

The SSR assertions in CI (Part 10) are really end-to-end tests of the
rendering contract.

---

## Part 10 — Deployment and operations

### 10.1 CI (`.github/workflows/main.yml`)

1. **`build` matrix, one job per Go module** (`fail-fast: false`, so one
   broken module doesn't hide the others): `go mod tidy` + `git diff
   --exit-code` (the module is tidy), `gofmt -l`, `go build`, `go vet`,
   golangci-lint v2 (action v8), `go test -short -race` (**the race
   detector**, which matters with goroutine janitors and caches).
2. **`migrations`**: a Postgres service container; for each service, create a
   test database, **`migrate up`, `down -all`, `up`**. Plus `docker compose
   config --quiet` with a throwaway key pair, to validate the compose file.
3. **`integration`** (`needs: [build, migrations]`): the testcontainers suites
   and the stack tests, with output teed to a file. A **"Failure summary"**
   step runs only on failure and reprints just the failing assertions at the
   end of the log (compose's container chatter buries them otherwise).
4. **`frontend`**: Node 22 with npm cache, `npm ci` (exact lockfile),
   `prettier --check`, `npm run build` (**the production build is also the
   template type check and enforces bundle budgets**; `tsc` alone doesn't
   check templates), `ng test`, then the **SSR assertions**: start
   `server.mjs`, wait for the port (no fixed sleep), then assert that `/`
   contains `<app-shell`, `og:title` and `rel="canonical"`; that `/account`
   is **not** server-rendered; that `/nope` returns **404**; and that `/clubs`
   returns **200**. Finally it builds the frontend Docker image (catches a
   Dockerfile drifting from the build).

### 10.2 Local development modes

- **Docker**: `make keys && make up` → everything on `:8000`.
  `make observability` adds Prometheus (`:9090`) and Jaeger (`:16686`).
- **No Docker** (my Windows machine): `scripts/dev-setup.ps1` (keys, roles and
  databases, migrations; idempotent), `scripts/dev-run.ps1 identity|football`
  (env from `.env`, services on their own ports with no gateway, which is why
  `apiConfigOf` exists), `scripts/seed-demo.ps1` (5 leagues × 3 clubs × 3
  players + coaches; **idempotent** by looking up by name first),
  `scripts/test.ps1` (enters each of the six modules, since `go test ./...`
  doesn't work from a workspace root).
- `docs/START-HERE.md`, `docs/DOCKER-WSL.md`, `docs/AUTH-SETUP.md`: step-by-step
  guides.

### 10.3 Production

| Piece | Where | Detail |
|---|---|---|
| Frontend | **Vercel** | Assets on the CDN; SSR as the `api/ssr.mjs` serverless function; rewrites act as the gateway. |
| identity-svc, football-svc | **Railway** | Built from the Dockerfiles with the repo root as context. `CLIENT_IP_HEADER` defaults to `X-Real-IP` when `RAILWAY_PROJECT_ID` is present; `/metrics` returns 404 without a token. |
| Databases | **Supabase Postgres** | Two databases; `DB_SSLMODE=require`. **Migrations are applied by hand** (see Part 12). |
| Events | NATS | Optional; **production currently runs without it** (see Part 12). |

### 10.4 Operations scripts

- `scripts/backup-db.ps1`: `pg_dump` of both production databases into a
  timestamped folder with a `manifest.json`. Credentials are pulled at run time
  through the Railway CLI, never stored.
- `scripts/restore-db.ps1`: restores one dump. Its first purpose is **proving a
  backup works** ("a backup that has never been restored is a hope"). A remote
  target needs `-AllowRemote` **and** typing the database name back.
- `scripts/check-endpoints.ps1`: an end-to-end API check (register, move a
  player through a real transfer, verify the temporal invariants; a malformed
  id is 400, an unauthenticated write is a JSON 401, an editor can't do an
  admin write).
- `smoke-test.sh` / `smoke-test.ps1`: validate compose, build, start, and check
  the containers.

---

## Part 11 — Concept glossary (concept → where it lives)

| Concept | Where in the code | One-line explanation |
|---|---|---|
| Layered architecture | every service | handler (transport) → service (rules + authz) → repository (SQL). |
| Repository pattern | `internal/repository`, `core/api/http` | Data access behind interfaces, so tests use fakes. |
| Dependency inversion | `main.go` wiring; Angular `InjectionToken`s | Depend on interfaces; one place binds implementations. |
| Interface segregation | `contracts.ts` readers/writers | A read-only page *can't* call writes. |
| Error taxonomy | `libs/platform/apperr` + `httpx.StatusFor` | Kinds, not status codes, in the domain; one mapping; safe messages. |
| Temporal modelling / derived state | migration 000003, transfer and value repositories | History is truth; current state is a cached projection kept in step inside transactions. |
| Pessimistic locking | `SELECT ... FOR UPDATE` in `Record` | Serialises concurrent transfers for one player. |
| Optimistic, predicate-based concurrency | `WHERE revoked_at IS NULL` / `used_at IS NULL` | Exactly one concurrent writer matches; the loser sees 0 rows. |
| Newest-wins guard | `NOT EXISTS (newer row)` | Backfilled history can't overwrite current state. |
| Idempotency | `external_refs`, upserts, `IF NOT EXISTS` migrations, idempotent event handlers, seed script | Running twice gives the same result. |
| Keyset vs offset paging | `domain/page.go` | Offset with a `limit+1` sentinel; no `COUNT`. Keyset would be next for deep pages. |
| Full-text search | migration 000007, `search_repository.go` | A generated weighted `tsvector` + GIN, `pg_trgm` similarity for ranking. |
| Exclusion constraint | migration 000005 | `EXCLUDE USING gist (daterange WITH &&)`: no overlapping seasons. |
| Partial index | migrations 000002, 000010 | Index only the rows a query touches (free agents, live articles). |
| Asymmetric JWT, JWKS, `kid` | `libs/auth` | Only the issuer can sign; keys discoverable and rotatable. |
| Algorithm pinning | `jwt.WithValidMethods` | Blocks `alg` confusion. |
| Refresh rotation + reuse detection | identity `Refresh`, `Rotate` | A single-use refresh token; replay → revoke all sessions. |
| Single-flight | `auth-interceptor.ts`, `LookupStore`, `PlayerNames` | Concurrent callers share one in-flight promise. |
| HttpOnly / SameSite cookies, CSRF | `session_cookie.go` | Script can't read the cookie; cross-site requests can't send or use it. |
| OAuth 2.0 code flow + PKCE + state | `oauth.go` | CSRF on the callback blocked; intercepted codes useless. |
| One-time code handoff | `oauth_login_codes` | No tokens in URLs. |
| Account enumeration defence | `Login` dummy hash | Same time and response whether the account exists or not. |
| Rate limiting (token bucket) | `ratelimit.go` | Per IP, per account, failures-only. |
| Slowloris | `ReadHeaderTimeout` | Header read deadline. |
| Graceful shutdown | `server.Run` | Drain in-flight requests on SIGTERM. |
| Correlation id | `RequestID` middleware + frontend interceptor | One id across logs, traces, errors and events. |
| Metric cardinality | `routeOf(r.Pattern)` | Label by route template, not path. |
| Distributed tracing | `tracing.go` | W3C propagation, parent-based sampling. |
| At-most-once events | `nats.go` core pub/sub | Fire-and-forget after commit; documented limitation. |
| Competing consumers | NATS queue groups | One replica handles each event. |
| SSR / hydration / TransferState | `app.routes.server.ts`, `ssr-resource.ts`, `transferred-map.ts` | Server HTML; the browser's first render must match it. |
| Soft 404 | `**` with `status: 404` | The real status code for unknown URLs. |
| CSP with hashes | `server.ts` | Only known inline scripts run. |
| Stored-XSS defence | `article_body.go`, `article-body.ts` | A structured document + allow-lists + no `innerHTML`. |
| Lazy loading / code splitting | `loadComponent`, `import('./engine')`, `@defer` | Pay for code only where it's used. |
| Immutable state + structural sharing | `editor-store.ts` | Cheap undo and change detection. |
| Pure core, imperative shell | `playback.ts`/`engine.ts`, the game engine | Logic testable without WebGL or Angular. |
| Money as integers | `domain.Minor`, `MoneyPipe`, `parseMoney` | No floats anywhere money is computed. |

---

## Part 12 — Weak spots to own before they are found

Saying these first, with the fix I'd make, is worth more than any feature.

1. **Production migrations are manual.** I've deployed code that queried
   columns that didn't exist yet, **three times** (000009, 000010, 000011):
   player pages returned 500 and players showed as "Unknown player". The
   importer now refuses to run without 000011 (`to_regclass` check). **Fix:**
   run `migrate up` as a release step, or at startup under a Postgres advisory
   lock so concurrent instances don't race.
2. **Events are at-most-once, and production runs without NATS.** Core NATS
   pub/sub (JetStream is enabled on the server but not used) after the commit
   means a crash between commit and publish, or a broker outage, loses the
   event. So today a deleted account's editor grants are **not** cleaned up in
   production. Also, the `Envelope.ID` comment in `events.go` says "delivery is
   at-least-once", which contradicts the README; I'd fix that comment. **Fix:**
   a **transactional outbox** (write the event row in the same transaction; a
   relay publishes to JetStream with acks; consumers dedupe by event id),
   which gives at-least-once with idempotent handlers.
3. **Rate limits and the grant cache are in-process.** Correct for one
   instance; with N instances the effective limit is N× and a revocation can be
   up to 5 s stale on other instances. **Fix:** Redis (or Postgres) for the
   buckets; the role-change and user-deleted events already exist to
   invalidate caches.
4. **`LookupStore` loads every club and competition.** Fine at ~240 clubs (I
   scoped the import to keep it small), but it doesn't scale. **Fix:** a batch
   `GET /teams?ids=` (the pattern `PlayerNames` already uses) or `?expand=`.
5. **Offset pagination** degrades on deep pages and can skip or repeat rows
   while data changes. **Fix:** keyset pagination on `(transfer_date, id)`.
6. **identity-svc has no service layer** (handlers → repositories). Acceptable
   for protocol logic; I'd add one when it gets real domain rules.
7. **The README is stale**: it still says "Phase 4 Frontend — not started"
   (README lines 13 and 197). Update it before sharing the repo.
8. **A frontend comment disagrees with the backend**: `admin/club-editors.ts`
   says an editor can't *rename* a club, but `teamService.UpdateTeam` lets an
   editor with a grant change everything except `league_id`. Decide which is
   intended and align them.
9. **No match data**: no fixtures, results or league tables. Season stats are
   imported aggregates, so they can't be corrected through the API.
10. **AI assistance.** I used Claude Code as a pair programmer for
    implementation, reviews and audits. Be ready to defend any line: the
    reasoning is in the code comments and `ISSUES.md`, and this document is
    what I'd use to show I own the design decisions.

---

## Part 13 — Tech-lead questions, with answers

**Why microservices for something this size?**
Identity and football data change for different reasons and carry different
risk: only identity holds passwords and the signing key, so a compromised
football service can't mint an admin token. Splitting them let me practise
cross-service auth and events. I kept the cost low with the shared platform
library and a service template. Honestly, for most products of this size I'd
start as a modular monolith with the same internal boundaries.

**Why Go's standard library router instead of Gin or chi?**
Go 1.22 added method and path-parameter patterns to `ServeMux`, and `r.Pattern`
gives the matched route for metrics and tracing. Middleware is just
`func(http.Handler) http.Handler`. A framework would add little.

**Why sqlx instead of an ORM like GORM?**
The interesting code is transactional: row locks, `NOT EXISTS` guards,
`LATERAL` joins, `unnest` bulk inserts, upserts. I wanted to see and control
every query. sqlx gives struct scanning without hiding SQL.

**How do you keep the squad consistent with transfer history?**
The transfer is the only write path for `players.team_id`. One transaction:
lock the player row (`FOR UPDATE`), re-check the origin, insert the transfer,
update the club only if no newer transfer exists, commit. Player updates
ignore `team_id`. A stack test proves a backdated transfer doesn't change the
current club.

**What happens if two editors record a transfer for the same player at once?**
The first takes the row lock; the second blocks, then re-reads the club inside
its transaction, sees it changed, and fails with "the player's club changed
while this transfer was being recorded; retry".

**Why RS256 instead of HS256?**
With a shared secret, every service that can verify a token can also forge
one. With RS256 only identity holds the private key; the others fetch public
keys from JWKS, matched by `kid`, so rotation needs no redeploy. The algorithm
is pinned during validation to prevent algorithm confusion.

**Where are tokens stored on the client, and why?**
The access token is in memory only (dies with the tab, so injected script can
only use it while the page is open). The refresh token is in an HttpOnly,
SameSite=Strict cookie scoped to the auth path, so script can't read it at all.
localStorage holds only a "signed in" hint. It used to hold the refresh token;
I migrated away and kept a one-time upgrade path.

**How do you handle the access token expiring mid-page?**
Concurrent 401s share one module-level refresh promise, then all retry.
Without that, reuse detection would see the same refresh token presented
several times and log the user out everywhere. On the server side, a 30-second
grace window handles two tabs refreshing at the same moment.

**How does a revoked editor grant take effect?**
Grants aren't in the token; the `Authorizer` reads them per request with a
5-second cache that's invalidated when a grant changes. Revocation is
immediate on the instance that handled it and at most 5 seconds stale
elsewhere. Role changes revoke refresh tokens, so they apply within one
15-minute access-token lifetime.

**How do you prevent XSS in user-written articles?**
The body is a ProseMirror JSON tree, never HTML. The server validates every
node type, mark and URL against allow-lists; the reader renders through
Angular templates with no `innerHTML` and re-checks URLs; and a hash-based CSP
blocks any inline script that isn't known.

**How did you make SSR work with authentication?**
Public pages render anonymously on the server, which is exactly what should be
indexed. Pages that need a session are `RenderMode.Client`, so guards never run
without a session. In the browser, an app initializer exchanges the HttpOnly
cookie for an access token before the first route renders.

**What was the hardest bug?**
Pick one and tell it in full:
- **SSR silently doing nothing**: the Host-header allow-list made Angular fall
  back to client rendering with a 200. I added a CI assertion on the rendered
  markup so it can't regress silently.
- **Cookies scoped to a path the browser never visits**: every reload signed
  the user out and OAuth failed as "expired" in under a millisecond, because
  the gateway stripped `/api/identity`. Fixed with a configured public path
  prefix.
- **Only the home page not server-rendered on Vercel**, because
  `index.csr.html` matched `/` before the rewrite.

**How is the importer safe to re-run?**
`external_refs` maps (source, kind, external id) to local UUIDs, so every row
is an upsert. It's one transaction, with `-dry-run` to see the report and roll
back. Matching by normalised names keeps hand-entered judgement and corrects
facts.

**How would you scale this?**
In order: run migrations automatically; keyset pagination; batch lookup
endpoints; move rate limits and caches to Redis; a transactional outbox with
JetStream; read replicas for the public reads; HTTP caching (ETags, CDN) for
public JSON; and only then split football-svc further (for example a separate
transfer-feed read service fed by events).

**What would you do differently from day one?**
The temporal model from the start (migration 000003 had to backfill), RS256
from the start, migrations as part of every deploy, and an outbox instead of
publish-after-commit.

**How do you know it works?**
214 Go tests, including race-detector runs, real-Postgres integration tests
and black-box compose-stack tests; 137 frontend specs; migrations rolled up,
down and up in CI; SSR assertions on real rendered HTML and status codes; and
an 82-item audit log with each fix recorded.


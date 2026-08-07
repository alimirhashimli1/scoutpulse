# ScoutPulse Micro — Issue Register

Audit date: 2026-08-07. Findings from a full read of the repository at commit `6ad2c79`.

Status legend: `[x]` fixed · `[ ]` open

**Verification:** all six modules build, vet, and pass `go test -short`. Docker is
not running on the audit machine, so the testcontainers suites skip locally; they
are wired to run for real in CI.

| Priority | Fixed | Open |
|---|---|---|
| P0 Blocking | 3 | 0 |
| P1 Security | 3 | 3 |
| P2 Correctness | 6 | 1 |
| P3 Architecture | 7 | 1 |
| P4 Infrastructure | 5 | 0 |
| P5 Platform | 2 | 6 |

---

## P0 — Blocking (repo did not build / CI was red)

### B1. `identity-svc` did not compile
- [x] **Files:** `apps/identity-svc/internal/handler/auth.go`, `internal/repository/postgres_user.go`
- **Problem:** Imported `football-database-app/...` while the module is declared `github.com/scoutpulse/identity-svc`. `go build ./...` failed with three *"package ... is not in std"* errors. The auth service could not be built or run, and the `identity-svc-ci` job failed.
- **Fixed:** imports rewritten to the `github.com/scoutpulse/...` paths already used by `cmd/server/main.go` and the test files.

### B2. Root `go.mod` was a phantom module
- [x] **File:** `go.mod` (deleted)
- **Problem:** `module football-database-app`, zero requirements, imported by nothing. It is what induced the broken paths in B1.
- **Fixed:** deleted and replaced by `go.work` (see F1).

### B3. `football-svc` failed `go vet` — missing test dependency
- [x] **File:** `apps/football-svc/internal/tests/football_svc_integration_test.go`
- **Problem:** imported `testcontainers-go`, absent from `go.mod`, so `go test ./...` failed.
- **Fixed:** dependency added. Adding it surfaced a second compile error (`coachSvc` declared and not used) — the handler is now wired with the real player and coach services instead of `nil, nil`.

---

## P1 — Security

### S1. Hardcoded JWT signing secret, committed to git
- [x] **File:** `libs/auth/auth.go`
- **Problem:** `var jwtSecret = []byte("scoutpulse_secret_key")`. Every environment shared one secret, public in the repository — anyone with repo access could forge an admin token against any deployment.
- **Fixed:** key loads from `JWT_SECRET` via `auth.LoadSecretFromEnv()`, called at startup in both services, fatal if unset. Minimum 32 bytes enforced. Token operations fail closed when no key is installed. Also pinned the signing algorithm with `jwt.WithValidMethods` to close algorithm-confusion, and added `sub`/`iss`/`iat` registered claims.

### S2. Privilege escalation via public registration
- [x] **File:** `apps/identity-svc/internal/handler/auth.go`
- **Problem:** `RegisterRequest` accepted a client-supplied `role` written straight to the database. `POST /api/v1/auth/register {"role":"admin"}` granted full write access over the entire dataset — a complete authorization bypass.
- **Fixed:** the `Role` field is gone from the payload; self-registration always assigns `model.UserRole`. `DecodeJSON` rejects unknown fields, so the smuggled key is refused outright. Regression test: `TestRegister_IgnoresClientSuppliedRole`. The integration test that previously *asserted* the escalation worked now asserts it does not. Added a minimum password length.

### S3. Internal error details leaked to clients
- [x] **Files:** all handlers in both services
- **Problem:** `http.Error(w, err.Error(), 500)` returned raw `lib/pq` messages — SQL fragments and constraint names — to the caller.
- **Fixed:** `libs/platform/apperr` splits the client-safe message from the internal cause; `httpx.WriteError` logs the cause and serializes only the message. Errors that never passed through `apperr` degrade to a generic `"internal server error"`. Regression tests: `TestWriteError_DoesNotLeakCause`, `TestWriteError_UnknownErrorStaysGeneric`, `TestHandlerDoesNotLeakInternalErrors`.

### S4. `managed_team_ids` embedded in the JWT
- [ ] **Files:** `libs/auth/auth.go`, `apps/identity-svc/internal/handler/auth.go`
- **Problem:** editor team grants are frozen into a 24-hour token. Granting a team takes effect only after re-login, and **revoking a team cannot take effect at all for 24 hours**. Token size grows without bound.
- **Fix:** keep `sub` + `role` in the token; resolve team grants server-side per request with a short-TTL cache. Depends on F4.

### S5. No token revocation, no refresh flow
- [ ] **File:** `libs/auth/auth.go`
- **Problem:** 24h expiry, no `jti`, no denylist, no refresh token. A leaked token is valid for a full day with no recourse.
- **Fix:** short-lived access tokens plus a refresh endpoint; `jti` and a revocation list.

### S6. Live API key in the working tree
- [ ] **File:** `.env`
- **Problem:** contains a real-looking `GEMINI_API_KEY`.
- **Verified:** correctly gitignored; `git log --all -- .env` confirms it was **never committed**.
- **Action for you:** rotate the key if it has ever left this machine. A committed `.env.example` now documents every required variable.

---

## P2 — Correctness & data integrity

### C1. Three mutually contradictory football schemas
- [x] **Problem:** `apps/football-svc/migrations/000001_...sql` (canonical, UUID PKs, matches the Go structs) coexisted with `apps/football-svc/db/schema.sql` (`SERIAL` PKs, different columns, no coaches table) and a third variant at the repo root. Applying the wrong one produced a database the code could not read.
- **Fixed:** both dead files deleted, along with the unused `apps/identity-svc/db/schema.sql`. Matching `.down.sql` files added for every migration.

### C2. Every repository error collapsed into `ErrNotFound`
- [x] **File:** `apps/football-svc/internal/service/player_service.go`
- **Problem:** `if err != nil { return ErrNotFound }` meant a database outage was reported to the client as `404 Player not found`. No repository mapped `sql.ErrNoRows`.
- **Fixed:** new `internal/repository/errors.go` translates `sql.ErrNoRows` → `NotFound`, unique violation → `Conflict`, FK/not-null violation → `Invalid`, everything else → `Internal` with the cause preserved for logs. A new `affected()` helper returns `NotFound` when an UPDATE or DELETE matches zero rows, so writes against a missing id no longer report success.

### C3. `market_value` is a binary float
- [ ] **File:** `apps/football-svc/internal/domain/models.go`
- **Problem:** `float64` against a `NUMERIC(15,2)` column — money in binary floating point accumulates rounding error.
- **Fix:** integer minor units (cents) or a decimal type. Deferred because it is a breaking wire-format change best done alongside A7.

### C4. `fmt.Sprintf` with no format arguments
- [x] **File:** `apps/football-svc/internal/repository/player_repository.go`
- **Fixed:** removed as part of the query-builder rewrite, which now numbers placeholders as arguments are appended so the WHERE clause and argument slice cannot drift apart.

### C5. Ignored `json.Encode` errors
- [x] **Fixed:** all response writing goes through `httpx.WriteJSON`, which logs encoding failures (the status line is already committed by then, so they cannot be reported to the client).

### C6. `main_test.go` tested nothing
- [x] **File:** `apps/football-svc/main_test.go` (deleted)
- **Problem:** built its own `ServeMux` inline, registered a handler returning a hardcoded list of three player names, then asserted on that hardcoded list. It exercised no application code and would have passed against an empty service.
- **Fixed:** deleted. Real coverage now lives in `internal/handler/handler_test.go`.

### C7. `tests/integration/stack_test.go` had never compiled
- [x] **Problem:** the module was outside the workspace and nothing ever built it. It used the same broken `football-database-app/...` import paths as B1; referenced compose services `identity-db`/`football-db` that do not exist (they are `postgres-identity`/`postgres-football`); used port 8080 for football-svc (it listens on 8081); connected to database `football` (it is `football_db`); called routes without the `/api/v1` prefix; asserted on client-invented IDs the schema generates server-side; and imported an `internal/` package across a module boundary, which Go forbids outright.
- **Fixed:** rewritten as a genuine black-box stack test with correct service names, ports, database name, and routes; IDs are read back from responses; local wire structs replace the illegal `internal/` import. Added `TestMigrationsApplied`, which asserts the golang-migrate jobs ran and the new indexes exist. The module is now in `go.work` and CI.

---

## P3 — Architecture & design

### A1. RBAC implemented four separate times, inconsistently
- [x] **Problem:** handlers hand-rolled role checks *and* the service layer re-checked via `footballAuthz`, and the copies disagreed — `CreateTeam` was admin-only in the handler, `CreatePlayer` ran the check twice, `CreateCoach` and `UpdateTeam` had their own inline chains the service never saw.
- **Fixed:** authorization lives in the service layer only; handlers are pure transport. The service layer's rules were already complete and correct, so the handler copies were pure redundancy. `internal/handler/common.go` documents the boundary. The handler test suite was reframed from asserting roles to asserting the handler renders each service outcome with the right status; role-by-role coverage stays in `internal/service/football_test.go`.

### A2. Dead interfaces in the route table
- [x] **File:** `apps/football-svc/internal/handler/routes.go`
- **Problem:** four interfaces declared with `http.ResponseWriter` signatures that nothing implemented and nothing referenced, shadowing the real names in `internal/service`. One had no writer parameter at all.
- **Fixed:** deleted.

### A3. No pagination on any list endpoint
- [x] **Problem:** `SELECT ... FROM players` with no `LIMIT` — at Transfermarkt scale, millions of rows per request.
- **Fixed:** `domain.Page` clamps client input (default 25, max 100); repositories fetch `limit+1` rows so `has_more` is known without a `COUNT` query; every list response is a `domain.ListResult[T]` envelope with `items`/`limit`/`offset`/`has_more`. Ordering is `(name, id)` so offset paging stays stable. **This changed the response shape of every list endpoint** — items are now under `items` rather than at the top level.

### A4. No indexes beyond primary keys and uniques
- [x] **Fixed:** migration `000002_add_lookup_indexes` adds indexes on `players.team_id` and `teams.league_id`, a partial index for free-agent lookups, an index on `players.position`, and `(name, id)` indexes so the list ordering is served without a sort.

### A5. No server hardening or lifecycle management
- [x] **Problem:** bare `http.ListenAndServe` with no timeouts (Slowloris-exposed), no graceful shutdown, no request logging, no CORS — and the missing CORS layer would have blocked the Angular frontend on day one. identity-svc used the global `DefaultServeMux`.
- **Fixed:** `libs/platform/server.Run` applies read/write/idle/header timeouts, a middleware chain (request ID, structured logging, CORS, panic recovery), and drains in-flight requests on SIGINT/SIGTERM. Both services use their own mux and the same bootstrap.

### A6. Migrations ran only on first volume initialization
- [x] **Problem:** schema was applied by mounting SQL into `docker-entrypoint-initdb.d`, which Postgres runs only when the data directory is empty. Adding a column required `make clean`, destroying all data.
- **Fixed:** dedicated `migrate/migrate` compose jobs run golang-migrate against a persistent `schema_migrations` table, gated on database healthchecks; services wait on `service_completed_successfully`. `make migrate` applies pending versions to an existing database without data loss.

### A7. Domain model has no temporal dimension
- [ ] **File:** `apps/football-svc/internal/domain/models.go`
- **Problem:** the model is a flat snapshot. There is no `transfers` table, no market-value history, no seasons. A transfer overwrites `players.team_id` and the previous fact is lost forever. **Transfermarkt *is* transfer history and value-over-time** — the current model cannot express the core product.
- **Also missing:** date of birth, nationality, height, preferred foot, agent, contract start, squad number, appearances/goals, injuries, national-team caps, staff beyond one head coach, and competitions distinct from leagues (a team plays in a league *and* a cup, *within a season*).
- **Fix:** redesign around seasons and event history. **Do this before frontend work** — it reshapes every API response. This is the largest remaining item and deserves its own design pass.

### A8. Missing endpoints for existing service methods
- [x] **Problem:** `GetPlayer` and `DeletePlayer` were implemented and unroutable; no `DELETE` route existed for any entity.
- **Fixed:** full CRUD routed for all four entities, including `GET /{id}` and `DELETE /{id}`. A `protect()` helper makes the public/authenticated split visible at a glance.

---

## P4 — Infrastructure & repo hygiene

### D1. Two forked `docker-compose.yml` files
- [x] **Fixed:** `deploy/docker-compose.yml` (which had drifted, missing football-svc and the frontend) deleted; the root file is authoritative. It now also has healthchecks, `${VAR}` substitution, and a required `JWT_SECRET`.

### D2. Frontend stub built by Compose
- [x] **Fixed:** moved behind a `frontend` compose profile, so `docker compose up` no longer builds an empty nginx image. Bring it up with `docker compose --profile frontend up` once Phase 4 starts.

### D3. CI had no lint job for `football-svc`
- [x] **Fixed:** CI is now a matrix over all five service/library modules running tidy-check, build, vet, lint, and race-enabled unit tests, plus a separate integration job where Docker is available. Adding a service is one line.

### D4. No linter configuration in the repo
- [x] **Fixed:** `.golangci.yml` added, enabling `errorlint`, `gosec`, `bodyclose`, `rowserrcheck`, `sqlclosecheck`, `noctx`, `revive` and more. `make lint` and `make test` now propagate failures instead of swallowing them behind `|| echo "not configured"`.

### D5. Aider working files cluttering the repo
- [x] **Fixed:** `.aider.chat.history.md` (188 KB), `.aider.input.history`, and `.aider.tags.cache.v4/` removed from the working tree. They were already gitignored.

---

## P5 — Scalability platform (prerequisites for adding more apps)

### F1. No Go workspace
- [x] **Fixed:** `go.work` lists all six modules. A new app is one line there and one in the Makefile's `MODULES`. Dockerfiles set `GOWORK=off` so images build from a single module's `go.mod`/`go.sum` and stay reproducible.

### F2. No shared service bootstrap — **highest-leverage item**
- [x] **Fixed:** new `libs/platform` module with three packages:
  - **`apperr`** — the error taxonomy (`Invalid`, `Unauthorized`, `Forbidden`, `NotFound`, `Conflict`, `Internal`), enforcing the split between the client-safe message and the internal cause.
  - **`server`** — `Run()` with hardened timeouts and graceful shutdown, plus `RequestID`, `Logging`, `CORS`, and `Recover` middleware.
  - **`httpx`** — `WriteJSON`, `WriteError` (the single error→status mapping in the codebase), `DecodeJSON` (size-capped, rejects unknown fields), and query-parameter helpers.

  A new service's `main.go` is now ~40 lines. This is what closed S3, C2, C5, and A5 in one place rather than per-service.

### F3. No API gateway
- [ ] **Problem:** the frontend targets `:8080`, `:8081`, and would need `:8082`, `:8083` … as apps are added.
- **Fix:** Traefik or Caddy in Compose routing `/api/identity/*`, `/api/football/*`, `/api/<new-app>/*`. Adding an app becomes one router rule with zero frontend changes, and gives one place for CORS, rate limiting, and TLS.

### F4. Symmetric JWT shared across services
- [ ] **Problem:** with HS256, every service that can *verify* a token can also *mint* one — including admin tokens. Dangerous the moment a third service exists.
- **Fix:** RS256 with a JWKS endpoint on identity-svc; other services verify with the public key only. Pair with `auth.RequireRole(...)` / `auth.RequireTeam(...)` middleware helpers so new services get RBAC declaratively.

### F5. One Postgres container per service
- [ ] **Problem:** does not survive six services on a laptop.
- **Fix:** one instance, one schema and one DB role per service. Preserves the isolation contract, cuts container count.

### F6. No event bus
- [ ] **Problem:** this is what actually enables adding small apps *without touching the core*. The only integration path specced is synchronous gRPC, which couples services tightly.
- **Fix:** NATS or Redis Streams. football-svc publishes `player.transferred`, `player.value_changed`; a notification app, transfer feed, search indexer, and analytics app each subscribe — and football-svc never learns they exist. Cheap at app #2, painful to retrofit at app #5.

### F7. No service template
- [ ] **Fix:** `apps/_template/` or `make new-service NAME=x`, so every app lands with the same layout, Dockerfile, health endpoint, and lint config.

### F8. Incomplete observability
- [ ] **Partly done:** structured `log/slog` request logging and `X-Request-ID` correlation (propagated from inbound headers so a call chain shares one ID) now ship in `libs/platform/server`, and request IDs are returned in every error body.
- **Still missing:** `/metrics` for Prometheus and OpenTelemetry tracing. Add before app #4.

---

## Recommended next steps

1. **A7 — the temporal domain redesign.** The largest remaining item and the one that decides whether this is a Transfermarkt clone or a CRUD app. Do it before any frontend work.
2. **S6** — rotate the Gemini key if it has left your machine.
3. **F4 then S4/S5** — asymmetric JWT unblocks proper grant and revocation handling.
4. **F3, F5** — gateway and database consolidation, before app #3.
5. **F6, F7, F8** — event bus, service template, metrics/tracing.
6. **C3** — fold the money-type change into the A7 schema work.

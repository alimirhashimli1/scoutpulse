# ScoutPulse Micro — Issue Register

Round 1 audit: 2026-08-07, at commit `6ad2c79`.
Round 2 audit: 2026-08-10, over the working tree at `410dd59` + uncommitted work.
Round 2 fixes applied: 2026-08-11.

Status legend: `[x]` fixed · `[ ]` open

**Verification:** all six modules build, vet, are gofmt-clean, and pass
`go test -short`. Docker is not available on this machine, so two things below
are **unverified locally and rely on CI**: migration `000005` has not been
applied to a real Postgres, and the `docker-compose.yml` changes have not been
parsed by `docker compose config`. Everything else was run.

| Priority | Fixed | Open |
|---|---|---|
| P0 Blocking | 3 | 0 |
| P1 Security | 5 | 1 |
| P2 Correctness | 7 | 0 |
| P3 Architecture | 8 | 0 |
| P4 Infrastructure | 5 | 0 |
| P5 Platform | 8 | 0 |
| **Round 1 total** | **36** | **1** |
| Round 2 | 21 | 0 |
| Round 2b | 10 | 0 |
| Round 2c (fallout) | 14 | 0 |
| **Total** | **81** | **1** |

S6 is a key rotation only you can perform.

Three round-2 items are fixed in the sense that matters but are worth knowing
the shape of — see [Partial fixes](#partial-fixes) for N7, N16 and N28.

---

## P0 — Blocking (repo did not build / CI was red)

### B1. `identity-svc` did not compile
- [x] Imported `football-database-app/...` while the module is declared `github.com/scoutpulse/identity-svc`. Three errors from `go build ./...`; the service could not run and CI was red.
- **Fixed:** imports rewritten to the paths already used by `cmd/server/main.go`.

### B2. Root `go.mod` was a phantom module
- [x] Zero requirements, imported by nothing, and the cause of B1.
- **Fixed:** deleted, replaced by `go.work` (F1).

### B3. `football-svc` failed `go vet` — missing test dependency
- [x] Imported `testcontainers-go`, absent from `go.mod`.
- **Fixed:** dependency added. Doing so revealed a second compile error underneath (`coachSvc` declared and not used); the handler is now wired with the real services instead of `nil, nil`.

---

## P1 — Security

### S1. Hardcoded JWT signing secret, committed to git
- [x] `var jwtSecret = []byte("scoutpulse_secret_key")` — one secret for every environment, public in the repository.
- **Fixed, then superseded by F4.** Signing is now asymmetric: the key comes from the environment, the algorithm is pinned against confusion attacks, and registered claims (`sub`/`iss`/`iat`/`jti`) were added.

### S2. Privilege escalation via public registration
- [x] `RegisterRequest` accepted a client-supplied `role` written straight to the database. `POST /register {"role":"admin"}` granted write access over the entire dataset.
- **Fixed:** the field is gone; self-registration always assigns `model.UserRole`, and `DecodeJSON` rejects unknown fields so the smuggled key is refused outright. Role changes moved to an admin-only `PUT /api/v1/users/{id}/role`, which also revokes the user's sessions. Regression test: `TestRegister_IgnoresClientSuppliedRole`. The integration test that previously *asserted the escalation worked* now asserts it does not.
- **Correction (N7, 2026-08-11):** this entry originally said the old role "cannot outlive the change". That overstates it. Revocation ends the user's *refresh* tokens; an access token already issued keeps the old role until it expires, so a demoted administrator retains their rights for up to 15 minutes. Accurate statement: **a role change takes effect within one access-token lifetime.**

### S3. Internal error details leaked to clients
- [x] `http.Error(w, err.Error(), 500)` returned raw `lib/pq` messages — SQL fragments and constraint names — to callers.
- **Fixed:** `apperr` splits the client-safe message from the internal cause; `httpx.WriteError` logs the cause and serialises only the message. Errors that never passed through `apperr` degrade to a generic string. Tests: `TestWriteError_DoesNotLeakCause`, `TestWriteError_UnknownErrorStaysGeneric`, `TestHandlerDoesNotLeakInternalErrors`.

### S4. `managed_team_ids` embedded in the JWT
- [x] Editor grants were frozen into a 24-hour token: a new grant needed a fresh login, and **a revocation could not take effect at all until the token expired**. The token also grew without bound.
- **Fixed:** grants moved to a `team_editors` table in the football service, which owns the clubs they refer to. They are resolved per request through `service.Authorizer`, behind a 5-second cache so the hot path stays off the database, with explicit invalidation on grant and revoke so both are immediate. Managed through `POST`/`DELETE /api/v1/teams/{id}/editors`. Tests: `TestAuthorizer_RevocationIsImmediate`, `TestAuthorizer_CachesGrantLookups`, `TestAuthorizer_InvalidateUserIsScoped`.

### S5. No token revocation, no refresh flow
- [x] 24h access tokens, no `jti`, no denylist, no refresh. A leaked token was usable for a day with no recourse.
- **Fixed:** access tokens are now 15 minutes and carry a `jti`. Long-lived access moved to an opaque refresh token with a server-side record, so it can actually be revoked. Refresh **rotates** the token, which gives leak detection: presenting an already-exchanged token means a copy is circulating, and every session for that user is revoked. Tokens are stored as SHA-256 hashes, so a database leak yields no live sessions. New endpoints: `POST /api/v1/auth/refresh`, `POST /api/v1/auth/logout`. Tests: `TestRefreshRotatesToken`, `TestRefreshReuseRevokesEverySession`, `TestLogoutRevokesToken`.

### S6. Live API key in the working tree
- [ ] **`.env` contains a real-looking `GEMINI_API_KEY`.**
- **Verified:** correctly gitignored; `git log --all -- .env` confirms it was **never committed**.
- **Action for you — the one item I cannot do:** rotate the key if it has ever left this machine. `.env.example` documents every variable the stack needs, and `make keys` generates the JWT key pair.

---

## P2 — Correctness & data integrity

### C1. Three mutually contradictory football schemas
- [x] The canonical migration coexisted with two dead files describing different tables. Applying the wrong one produced a database the code could not read.
- **Fixed:** both deleted, along with an unused `identity-svc/db/schema.sql`. Every migration now has a matching `.down.sql`, and CI applies, rolls back, and re-applies the full set.

### C2. Every repository error collapsed into `ErrNotFound`
- [x] A database outage was reported to the client as `404 Player not found`.
- **Fixed:** `repository/errors.go` maps `sql.ErrNoRows` → `NotFound`, unique violation → `Conflict`, FK/not-null → `Invalid`, everything else → `Internal` with the cause kept for logs. `affected()` returns `NotFound` when an UPDATE or DELETE matches zero rows, so writes against a missing id no longer report success.

### C3. `market_value` was a binary float
- [x] `float64` against `NUMERIC(15,2)` — decimal currency in binary floating point, with error compounding through every sum.
- **Fixed:** new `domain.Minor` integer type in minor units, with the column migrated and existing values carried across. It serialises as an integer (accepting a quoted form for JavaScript clients, which lose precision above 2^53) and **rejects decimals rather than truncating them**, since `1.5` cents is ambiguous. `TestMinor_NoFloatDrift` demonstrates the drift the type prevents.

### C4. `fmt.Sprintf` with no format arguments
- [x] Removed in the query-builder rewrite, which now numbers placeholders as arguments are appended so the WHERE clause and argument slice cannot drift apart.

### C5. Ignored `json.Encode` errors
- [x] All response writing goes through `httpx.WriteJSON`, which logs encoding failures.

### C6. `main_test.go` tested nothing
- [x] It built its own mux, registered a handler returning three hardcoded names, and asserted on those names. It would have passed against an empty service.
- **Fixed:** deleted; real coverage lives in `internal/handler/handler_test.go`.

### C7. `tests/integration/stack_test.go` had never compiled
- [x] Outside the workspace and never built. It had B1's broken imports; referenced compose services that do not exist; used the wrong port and database name; omitted the `/api/v1` prefix; asserted on client-invented IDs the schema generates; and imported an `internal/` package across a module boundary, which Go forbids outright.
- **Fixed:** rewritten as a real black-box test, now in `go.work` and CI, covering the ON DELETE SET NULL behaviour, migrations having run, transfer history, and that football-svc rejects a token signed by an untrusted key.

---

## P3 — Architecture & design

### A1. RBAC implemented four times, inconsistently
- [x] Handlers hand-rolled role checks *and* the service layer re-checked, and the copies disagreed.
- **Fixed:** authorization lives only in `service.Authorizer`; handlers are pure transport. `internal/handler/common.go` documents the boundary.

### A2. Dead interfaces in the route table
- [x] Four interfaces with `http.ResponseWriter` signatures that nothing implemented, shadowing the real names. Deleted.

### A3. No pagination on any list endpoint
- [x] `SELECT ... FROM players` with no `LIMIT`.
- **Fixed:** `domain.Page` clamps input (default 25, max 100); repositories fetch `limit+1` so `has_more` needs no `COUNT`; every list returns a `ListResult[T]` envelope. **This changed the shape of every list response** — items are under `items` rather than at the top level.

### A4. No indexes beyond primary keys
- [x] Migration `000002` adds foreign-key, partial free-agent, position, and `(name, id)` ordering indexes; `000003` adds the transfer, valuation, and spell indexes.

### A5. No server hardening or lifecycle management
- [x] `libs/platform/server.Run` applies read/write/idle/header timeouts, a middleware chain (request ID, structured logging, CORS, panic recovery), and drains in-flight requests on SIGINT/SIGTERM.

### A6. Migrations ran only on first volume initialization
- [x] Schema came from `docker-entrypoint-initdb.d`, which runs only on an empty data directory — so any change meant `make clean` and losing the data.
- **Fixed:** `migrate/migrate` compose jobs run golang-migrate against a `schema_migrations` table, gated on health checks, with services waiting on `service_completed_successfully`.

### A7. Domain model had no temporal dimension — **the largest item**
- [x] The model was a flat snapshot. A transfer overwrote `players.team_id` and the previous fact was gone. Transfermarkt *is* transfer history and value-over-time, so the model could not express the product.
- **Fixed** (migration `000003`), built around one rule — **history is the source of truth, current state is derived**:
  - **`transfers`** — every move, with type, fee (nullable for undisclosed, distinct from free), date and season. `players.team_id` is now maintained *by* the transfer flow, in the same transaction as the transfer row, so the squad can never contradict the history.
  - **`player_market_values`** — valuation over time, one entry per player per day. `players.market_value_minor` follows the latest, and backfilling an older figure will not overwrite a newer one.
  - **`seasons`** and **`team_seasons`** — the annual window everything else is stated against, and which competitions a club contested in each.
  - **`coach_spells`** — managerial history. The `UNIQUE` constraint on `coaches.team_id`, which made it impossible to record that a club ever changed manager, is gone.
  - **Player and club detail** — date of birth, nationality and second nationality, height, preferred foot, agent, squad number, contract start; stadium, city, country, founding year, short name.
  - **Competitions** — `tier` and `competition_type`, so a league, a domestic cup and a continental cup are all expressible.
  - Existing rows are backfilled with an opening transfer and valuation, so history is not silently empty for pre-existing data.

  Endpoints: `GET/POST /api/v1/transfers`, `GET /api/v1/players/{id}/transfers`,
  `GET/POST /api/v1/players/{id}/market-values`, `GET /api/v1/coaches/{id}/spells`,
  `GET /api/v1/seasons`, `/seasons/current`.

### A8. Missing endpoints for existing service methods
- [x] `GetPlayer` and `DeletePlayer` were unroutable; no entity had a `DELETE`. Full CRUD is now routed for all entities.

---

## P4 — Infrastructure & repo hygiene

### D1. Two forked `docker-compose.yml` files
- [x] `deploy/docker-compose.yml` had drifted and was missing football-svc entirely. Deleted; the root file is authoritative.

### D2. Frontend stub built by Compose
- [x] Moved behind a `frontend` profile.

### D3. CI had no lint job for `football-svc`
- [x] CI is a matrix over all six modules running tidy-check, gofmt-check, build, vet, lint and race-enabled tests, plus a migrations job and an integration job.

### D4. No linter configuration
- [x] `.golangci.yml` added with `errorlint`, `gosec`, `bodyclose`, `rowserrcheck`, `sqlclosecheck`, `noctx`, `revive` and more. `make lint`/`make test` propagate failures instead of swallowing them.

### D5. Aider working files cluttering the repo
- [x] Removed from the working tree.

---

## P5 — Scalability platform

### F1. No Go workspace
- [x] `go.work` lists every module. Dockerfiles set `GOWORK=off` so images build from a single module and stay reproducible.

### F2. No shared service bootstrap — **highest-leverage item**
- [x] New `libs/platform`: `apperr` (error taxonomy), `server` (bootstrap, middleware, graceful shutdown), `httpx` (JSON, the single error→status mapping, query helpers), `events`, `observability`. A new service's `main.go` is ~40 lines. This closed S3, C2, C5 and A5 in one place rather than per service.

### F3. No API gateway
- [x] Caddy on `:8000` routing `/api/identity/*` and `/api/football/*`, plus `/.well-known/jwks.json`. Adding a service is one route block and no frontend change. It also assigns the correlation id at the edge, so one id spans the whole chain.

### F4. Symmetric JWT shared across services
- [x] With HS256, every service that could *verify* a token could also *mint* one, including an admin token.
- **Fixed:** RS256. identity-svc holds the private key and publishes `/.well-known/jwks.json`; every other service is given only the public key. `TestVerifierCannotMint` and the stack test `TestFootballServiceCannotMintTokens` assert the property. `kid` headers and multiple trusted keys make rotation possible without downtime (`TestKeyRotation`).

### F5. One Postgres container per service
- [x] One instance with a database and login role per service, bootstrapped by `deploy/postgres/init-databases.sh`. Isolation is preserved; the container count is not.

### F6. No event bus
- [x] NATS, with `libs/platform/events` defining the subjects and payloads. football-svc publishes `player.transferred`, `player.value_changed`, `player.created`, `coach.appointed`, `team.created`, `team.deleted`. A new app subscribes instead of the core service being changed to call it.
- Publishing is wrapped in `SafePublisher`: the write it describes is already committed, so a transport failure is logged and counted rather than reported to the client as a failed transfer. With no `NATS_URL` set — or with the broker unreachable at startup (N27) — the publisher is a no-op and the service runs normally.
- **Delivery guarantee (N28), read before building on this.** The bus is
  **at-most-once**. Events are published after the commit, outside the
  transaction, and a failure is swallowed by design — so a crash between commit
  and publish, or a broker outage, drops the event permanently. There is no
  outbox, no retry and no redelivery, and although the NATS server runs with
  `--jetstream`, the publisher uses core NATS, so nothing is persisted either.
  **A subscriber can safely drive notifications or cache invalidation; it
  cannot drive a projection or read model that has to stay correct**, because
  it will silently miss writes. Alert on
  `events_published_total{outcome="error"}` to see the drops. A transactional
  outbox is the fix when a subscriber needs more than this.

### F7. No service template
- [x] `apps/_template/` plus `make new-service NAME=x PORT=y`, which scaffolds the service, wires `go.work` and the Makefile, and prints the remaining manual steps.

### F8. No observability
- [x] Prometheus `/metrics` on every service (request counts, latency histograms, in-flight gauge, event-publish outcomes — all labelled by **route pattern**, not raw path, to keep cardinality bounded), and OpenTelemetry tracing with W3C context propagation. Both optional: with no collector configured, tracing is a no-op. `docker compose --profile observability up` brings up Prometheus and Jaeger.

---

## Notes on what changed behaviourally

Worth knowing before you next run the stack:

1. **Setup now needs `make keys`** once, to generate the JWT key pair into `.env`.
2. **Login returns a different shape**: `{access_token, refresh_token, token_type, expires_in}` rather than `{token}`. Access tokens expire in 15 minutes; clients must use the refresh endpoint.
3. **List endpoints return an envelope**, not a bare array.
4. **Money is an integer count of minor units.** `market_value` became `market_value_minor`.
5. **A player's club changes only through a transfer.** `PUT /players/{id}` no longer moves a player between clubs, by design — that would bypass the history.
6. **Editor grants are no longer in the token.** Grant them with `POST /api/v1/teams/{id}/editors`.
7. **Everything is reachable through the gateway on `:8000`.** Direct service ports are still exposed for local debugging.

## Remaining work (not defects)

- **S6** — rotate the Gemini key if it has left your machine.
- **Phase 4, the Angular frontend.** The API is now stable enough to build against: the temporal model that would have reshaped every response is in place.
- Worth considering as the dataset grows: search (Postgres FTS or Meilisearch) for player and club lookup, a Redis cache for hot reads, and materialised views for league tables. None is needed yet.

---

# Round 2 — findings from the 2026-08-10 audit

A second pass over the working tree, including the temporal-domain, gateway,
events and observability work that was untracked at the time of round 1. All 21
are open. Nothing here is a build or test failure; several are cases where a
code comment or a round-1 entry claims a property the code does not actually
have, which is the kind of gap that survives review precisely because the
intent is documented.

## R0 — Broken CI

### N1. CI pins Go 1.24 but three modules require 1.25
- [x] `.github/workflows/main.yml` sets `GO_VERSION: "1.24"`. `go.work`, `apps/football-svc`, `apps/identity-svc` and `libs/platform` all declare `go 1.25.0`; `libs/auth`, `libs/db` and `tests/integration` declare `go 1.24.0`.
- The build steps survive only because `GOTOOLCHAIN=auto` silently downloads 1.25 — so CI is not testing the toolchain it says it is, and every job pays a toolchain download.
- **Fix:** raise `GO_VERSION` to `1.25` and make the seven `go` directives agree.

### N2. golangci-lint v1.64.5 cannot read a `go 1.25.0` module
- [x] The lint step pins `version: v1.64.5`, which predates Go 1.25 and rejects the language version in `go.mod` before it lints anything.
- Combined with N1 this means the lint gate that D4 introduced is not actually guarding the three largest modules. Worth confirming by looking at a recent Actions run before assuming those modules are lint-clean.
- **Fix:** move to a golangci-lint release built against Go 1.25.

## R1 — Security

### N3. `/metrics` is reachable through the gateway despite the rule meant to block it
- [x] The Caddyfile has `handle /metrics { respond "not found" 404 }` with the comment *"Exposing them at the edge would publish request volumes and latency profiles to anyone who asks."* That matcher only covers the literal path `/metrics`.
- `handle_path /api/football/*` strips its prefix, so **`GET :8000/api/football/metrics`** proxies straight through to football-svc's Prometheus endpoint. Same for `/api/identity/metrics`.
- **Fix:** block the suffix inside each service block (`handle /api/football/metrics { respond 404 }` before the `reverse_proxy`, or a `not path */metrics` matcher on the proxy).

### N4. No rate limiting anywhere, and `POST /auth/login` is the target
- [x] The Caddyfile comment says the gateway "is also the single place for CORS, rate limiting and TLS". CORS is in the services, TLS is unconfigured, and rate limiting does not exist at all.
- Login is unthrottled with no lockout and no backoff. It is also a cheap CPU-exhaustion vector: every attempt, valid or not, runs bcrypt at `DefaultCost`.
- **Fix:** a rate-limit module at the gateway keyed on client IP for `/api/identity/api/v1/auth/*`, plus per-account failure tracking in identity-svc.

### N5. Login leaks account existence through timing
- [x] `Login` returns immediately when `GetByIdentifier` misses, and runs bcrypt (~60–100 ms) when it hits. The response bodies are identical, and the comment above it claims *"the endpoint cannot be used to enumerate accounts"* — but the timing difference does, reliably and at scale.
- **Fix:** compare against a fixed dummy hash on the miss path so both branches cost the same.
- File: [auth.go:114-125](apps/identity-svc/internal/handler/auth.go#L114-L125)

### N6. `MinRSAKeyBits` is enforced only on the signer
- [x] `SetSigningKey` rejects keys under 2048 bits. `AddVerificationKey` and `JWK.toRSAPublicKey` do not check at all, so a 512-bit key arriving via `JWT_PUBLIC_KEY` or a JWKS document is installed and trusted for verification.
- A verification key that small is factorable, which means forgeable admin tokens. `LoadJWKS` also accepts a plain-`http` URL, so the key set is substitutable by anyone on the path.
- **Fix:** apply the same bit-length check in `AddVerificationKey` and `toRSAPublicKey`; require `https` in `LoadJWKS` unless the host is explicitly trusted.
- File: [keys.go:74-89](libs/auth/keys.go#L74-L89)

### N7. A role change does not take effect for up to 15 minutes
- [x] S2 states role changes are immediate because sessions are revoked. Revocation only ends *refresh* tokens; the access token the user is already holding keeps the old role until it expires.
- A demoted administrator therefore keeps administrator rights on every service for the remainder of that token's lifetime. 15 minutes is a defensible window, but it is not what S2 claims, and there is no `jti` denylist to close it.
- **Fix:** either soften the S2 claim to "within one access-token lifetime", or have services consult a short-TTL revoked-`jti` set — the `jti` is already minted and unused.

### N8. Postgres and NATS are published on all interfaces with default credentials
- [x] `docker-compose.yml` maps `5432:5432`, `4222:4222`, `8222:8222`, `8080:8080` and `8081:8081` to the host. `.env.example` ships `IDENTITY_DB_PASSWORD=password` and `FOOTBALL_DB_PASSWORD=password` as working defaults, and NATS runs with no authentication.
- The service ports also bypass the gateway entirely, which is what makes N3 and N4 worse: any control added at the edge is optional.
- **Fix:** bind the infrastructure ports to `127.0.0.1:` and drop the service port mappings behind a `debug` profile.

### N9. CORS fails open when `CORS_ALLOWED_ORIGINS` is unset
- [x] `allowedOriginsFromEnv` returns `["*"]` for an unset or empty variable. A deployment that forgets the variable gets a wide-open API rather than a broken one.
- Credentials are correctly withheld in the wildcard branch, so this is a defence-in-depth issue rather than a session-theft one.
- **Fix:** default to deny (or to nothing at all, and fail startup) and require the wildcard to be spelled out.
- File: [middleware.go:136-153](libs/platform/server/middleware.go#L136-L153)

## R2 — Correctness & data integrity

### N10. `DELETE /players/{id}/market-values/{valueID}` ignores `{id}`
- [x] The handler passes only `valueID` to the service, which passes it straight to `DELETE ... WHERE id = $1`. Any player id in the path deletes the row, so the URL asserts a relationship nobody checks.
- **Fix:** scope the delete to the player: `WHERE id = $1 AND player_id = $2`.
- File: [market_value_handler.go:61-67](apps/football-svc/internal/handler/market_value_handler.go#L61-L67)

### N11. Deleting a valuation leaves `players.market_value_minor` pointing at the deleted row
- [x] `Record` goes to real trouble to keep the derived column correct, including the `NOT EXISTS` guard so a backfill cannot overwrite a newer figure. `Delete` is a bare `DELETE` — remove the most recent valuation and the player keeps showing it, now with no history entry behind it.
- **Fix:** in the same transaction, reset the player to the newest surviving valuation (or to 0 when none remains).
- File: [market_value_repository.go:78-81](apps/football-svc/internal/repository/market_value_repository.go#L78-L81)

### N12. `CreatePlayer` writes current state with no history behind it
- [x] Migration `000003` states the rule plainly: *"transfers is the source of truth for where a player has played; players.team_id is derived"*, and it backfills an opening transfer and valuation for every pre-existing row precisely because that matters.
- `CreatePlayer` inserts `team_id` and `market_value_minor` directly and creates neither row. Every player added through the API since the migration reintroduces exactly the gap the backfill closed: a squad membership with an empty career history, and a current valuation absent from the valuation chart.
- **Fix:** create the opening transfer and valuation in the same transaction as the player, mirroring what the migration does.
- File: [player_repository.go:104-117](apps/football-svc/internal/repository/player_repository.go#L104-L117)

### N13. A backdated open coach spell silently becomes the current appointment
- [x] `Record` sets `coaches.team_id` whenever the new spell has no end date, with no comparison against the existing spell's dates. Recording a coach's 2015 spell (end date omitted) after their current one has been closed makes them the current coach of the 2015 club.
- The market-value path guards against exactly this shape of mistake; this one does not.
- There is a second failure mode: if the current spell is still open, `closePrevious` sets its `end_date` to the backdated `start_date`, which violates `coach_spells_dates_ordered` and surfaces as an opaque constraint error rather than a clear 400.
- **Fix:** only touch `coaches.team_id` when the new spell starts on or after every existing spell; reject a backdated spell that would close a later one.
- File: [coach_spell_repository.go:56-77](apps/football-svc/internal/repository/coach_spell_repository.go#L56-L77)

### N14. `coach_spells.role` accepts any string
- [x] `transfer_type`, `preferred_foot` and `competition_type` each have a `CHECK` constraint plus a `Valid*` helper in `domain`. `role` has neither — `RecordSpell` only defaults an empty value to `head_coach`, so `"role": "asdf"` is stored.
- **Fix:** a `ValidSpellRole` helper and a matching `CHECK`, following the pattern the other three already set.

### N15. `RecordTransfer` re-checks nothing inside the transaction
- [x] `checkOrigin` reads the player outside the transaction, and `Record` then updates `players.team_id` unconditionally. Two concurrent transfers for the same player both pass validation and both commit; the second wins the squad update and the history records two moves from the same origin.
- The same read-then-write gap means a backdated transfer that happens to match the current club still overwrites `players.team_id`.
- **Fix:** re-read the player `FOR UPDATE` inside the transaction and make the squad update conditional on `team_id` still matching `from_team_id`, plus on the date being the latest for that player.
- File: [transfer_repository.go:100-131](apps/football-svc/internal/repository/transfer_repository.go#L100-L131)

### N16. `Grant` validates the club but never the user
- [x] `TeamEditorService.Grant` calls `teams.GetByID` specifically so a bad club id returns a clean 404, then writes an arbitrary `user_id` with no check at all. A typo'd UUID is accepted silently.
- Two consequences: the grant is invisible dead data, and nothing removes grants when an account is deleted in identity-svc — the boundary comment in migration `000004` explains why there is no FK, but nothing was put in its place.
- Related: granting to a user whose global role is `user` (not `editor`) is also accepted and does nothing, because `RequireTeam` rejects on role before it ever looks at the grant.
- **Fix:** verify the user through identity-svc (or accept the grant only for a known editor), and subscribe to a user-deleted event to clean up.
- File: [team_editor_service.go:61-87](apps/football-svc/internal/service/team_editor_service.go#L61-L87)

### N17. Seasons may overlap, and `Current` then picks one arbitrarily
- [x] `seasons` constrains only `end_date > start_date`. Nothing stops two overlapping rows, and `Current` resolves the ambiguity with `ORDER BY start_date DESC LIMIT 1` — so a transfer gets attached to whichever season sorts first, silently.
- **Fix:** an exclusion constraint on the date range (`EXCLUDE USING gist (daterange(start_date, end_date) WITH &&)`), or an explicit overlap check in `validateSeason`.

## R3 — API consistency

### N18. `AuthMiddleware` returns plain text where everything else returns JSON
- [x] Three `http.Error` calls emit `text/plain` 401s. Every other error in both services goes through `httpx.WriteError` and returns `{"error", "code", "request_id"}`. A client with a single error parser breaks on the one status it is most likely to hit.
- The same applies to `server.Recover`, which writes a plain-text 500 on a panic.
- **Fix:** give `libs/auth` a JSON writer, or move the middleware behind `httpx`.
- File: [auth.go:159-182](libs/auth/auth.go#L159-L182)

### N19. JWKS is fetched once at startup and never refreshed
- [x] Three comments promise otherwise: *"rotation then needs no redeploy"* (`keys.go`), *"a rotation propagates without redeploying every consumer"* and *"consumers refetch on an unrecognised key id anyway"* (`jwks.go`).
- None of it is implemented. `LoadJWKS` runs once from `LoadFromEnv`; `keyForToken` returns an error for an unknown `kid` and never triggers a refetch; nothing schedules a refresh. Rotation still requires restarting every consumer — and in the compose stack nothing sets `JWKS_URL` at all, so football-svc uses the pinned `JWT_PUBLIC_KEY` and the JWKS endpoint has no consumers.
- **Fix:** refetch on unknown `kid` (rate-limited), plus a background refresh on the `Cache-Control` interval. Then point football-svc at `JWKS_URL` in compose so the path is actually exercised.
- File: [jwks.go:70-111](libs/auth/jwks.go#L70-L111)

## R4 — Hygiene

### N20. `refresh_tokens` grows without bound
- [x] `DeleteExpired` is implemented, is on the interface, and is stubbed in the test fake — but nothing calls it. Rotation writes a new row on every refresh, so an active user generates a row every 15 minutes and none are ever removed. Expired and revoked rows accumulate forever.
- **Fix:** run it from a ticker in identity-svc's `main`, or as a compose one-shot.
- File: [refresh_token.go:129-135](apps/identity-svc/internal/repository/refresh_token.go#L129-L135)

### N21. Small leftovers
- [x] `internal/domain/` at the repository root is an empty directory left over from the pre-workspace layout — delete it.
- [x] `DecodeJSON` does not check for trailing content, so `{"a":1}{"b":2}` decodes the first object and ignores the rest. One `dec.More()` check after `Decode`.
- [x] `Minor.UnmarshalJSON` rejects `null` with a confusing message rather than treating it as absent, which is the Go convention. Affects non-pointer fields like `market_value_minor`.
- [x] The `000003` down migration does not restore the `coaches.team_id` UNIQUE constraint that the up migration drops, so down-then-up is not a true round trip. CI's `up / down -all / up` cycle cannot catch it.
- [x] `.env.example` documents neither `JWKS_URL` nor `NATS_URL`, both of which the code reads.
- [x] `docker-compose.yml` gives neither service a healthcheck, so the gateway's `depends_on` waits for container start rather than readiness and returns 502s during startup.

---

# Round 2b — second sweep

The first round-2 pass covered the football domain, identity, auth and the
gateway. This one covers what was left: `libs/db`, `libs/platform/{server,
events,observability,apperr}`, the repository error mapping, the Dockerfiles
and the Postgres bootstrap.

### N22. Any malformed id returns 500 — and every CHECK constraint does too
- [x] `translate` maps `sql.ErrNoRows`, `23505`, `23503` and `23502`. It does not map **`22P02`** (invalid text representation) or **`23514`** (check violation), so both fall through to `apperr.Internal`.
- `GET /api/v1/players/not-a-uuid` therefore returns **500 Internal Server Error** and writes an error line to the log. Any client with a typo, any crawler, any fuzzer trips it. It should be a 400.
- Every `CHECK` the schema carefully defines — `transfers_type_valid`, `leagues_competition_type_valid`, `players_preferred_foot_valid`, `coach_spells_dates_ordered`, the non-negative money constraints — reports as a 500 rather than a 400 when the service-layer validation is bypassed or disagrees. This is also what makes N13's failure mode opaque.
- **Fix:** add both codes to the switch — `22P02` → `KindInvalid` ("malformed identifier"), `23514` → `KindInvalid`.
- File: [errors.go:36-44](apps/football-svc/internal/repository/errors.go#L36-L44)
- C2 in round 1 fixed the inverse of this bug (everything collapsing to `NotFound`); the mapping it introduced is just incomplete.

### N23. No database connection pool configuration at all
- [x] `Connect` calls `sqlx.Open` and never touches `SetMaxOpenConns`, `SetMaxIdleConns` or `SetConnMaxLifetime`. The defaults are unlimited open connections, **two** idle, and connections that live forever.
- Two services share one Postgres whose default `max_connections` is 100. Under load each service opens connections without bound until the server starts refusing them with `FATAL: sorry, too many clients already` — and in the meantime the idle cap of 2 means most requests pay a fresh TCP + TLS + auth handshake.
- `server.go` sets five HTTP timeouts with a comment explaining each. The database half of the same shared bootstrap (F2) got none of that attention.
- **Fix:** set the three pool limits in `Connect`, driven by environment variables with sane defaults.
- File: [db.go:33-48](libs/db/db.go#L33-L48)

### N24. The DSN is built by string concatenation, with TLS hardcoded off
- [x] `sslmode=disable` is baked into the format string and cannot be configured, so no deployment can encrypt its database traffic without editing this library.
- The password is interpolated with `%s` and never escaped. A password containing a space or a quote either breaks the DSN or injects another connection parameter into it.
- The same class of bug is in `init-databases.sh`, which interpolates into `CREATE ROLE ${role} LOGIN PASSWORD '${password}'` — running as the superuser at first initialisation.
- **Fix:** build the DSN with `net/url` (or `pq.ParseURL`), make `sslmode` configurable, and pass the bootstrap passwords through `psql -v` variables rather than shell interpolation.

### N25. `db.Ping()` has no timeout, and one log line escapes the structured stream
- [x] `Ping` rather than `PingContext`, with no deadline and no retry — startup hangs on an unresponsive database instead of failing or backing off. `noctx` should be flagging this; see N2 for why it may not be running.
- `log.Printf("Connected to database…")` writes a plain line into a stream that is otherwise slog JSON, so it will not parse in a log aggregator.

### N26. The event-publish alarm is registered but never fires
- [x] `observability.RecordEventPublished` exists, `events_published_total` is registered with an `outcome` label, and the function's own comment says *"this counter is the signal that the bus is failing"*.
- **Nothing calls it.** The counter is permanently zero. `SafePublisher` swallows the error after logging it and never increments anything.
- F6 claims a publish failure is "logged and counted". Only the first half is true, and the counter you would alert on is the half that is missing.
- **Fix:** call `RecordEventPublished` from `SafePublisher.Publish`. There is no import cycle — `events` already imports `server`, and `observability` does too.
- File: [nats.go:150-158](libs/platform/events/nats.go#L150-L158)

### N27. football-svc refuses to start when NATS is unreachable
- [x] Three comments state the principle: events are *"a secondary effect of a write, not part of it"*, and *"a service must still start and serve requests when the broker is absent"*.
- The code does the opposite whenever `NATS_URL` is set — which it always is in compose. `PublisherFromEnv` returns `Connect`'s error, and `main.go` turns it into `log.Fatalf("Failed to connect to the event bus")`. A NATS outage takes the entire football service down with it, including every read path that has nothing to do with events.
- The no-op fallback only covers the case where the variable is *unset*, which is the case that was never the risk.
- **Fix:** log and fall back to `NopPublisher` on a dial failure, and let the NATS client's existing infinite reconnect pick the broker back up.
- File: [main.go:44-47](apps/football-svc/main.go#L44-L47)

### N28. The event bus is at-most-once, with silent loss
- [x] Events are published after the transaction commits, outside it, with the error swallowed. A crash between commit and publish, or any broker outage, drops the event permanently — no outbox, no retry, no redelivery.
- The NATS server runs with `--jetstream`, but `Publish` uses core NATS, so none of that persistence is engaged. The publish is fire-and-forget on both ends.
- This bounds what F6's premise actually supports: a subscriber can drive notifications or a cache warm, but not a projection or read model that has to stay correct, because it will silently miss writes.
- **Fix:** if subscribers ever need reliability, a transactional outbox table drained by a relay is the standard answer. Until then, say so explicitly in the F6 entry so nobody builds a projection on it.

### N29. All three Dockerfiles pin Go 1.24 against `go 1.25.0` modules
- [x] `apps/football-svc/Dockerfile`, `apps/identity-svc/Dockerfile` and `apps/_template/Dockerfile.tmpl` all use `FROM golang:1.24-alpine`. Both service modules declare `go 1.25.0`, and `GOWORK=off` does not change that — the `go.mod` directive still governs.
- The build only succeeds by silently downloading a 1.25 toolchain at image-build time, which defeats the reproducibility the `GOWORK=off` comment is there to protect, and fails outright in a build without egress to the module proxy.
- `apps/_template/go.mod.tmpl` also declares `go 1.24.0`, so every scaffolded service starts on the older version.
- **Fix:** same as N1 — pick one Go version and apply it to the workflow, the three Dockerfiles, the template, and all seven `go.mod` files.

### N30. An editor can move their own club into any league
- [x] `UpdateTeam` authorizes with `RequireTeam(ctx, team.ID)` — the club's own editor — and `validateTeam` checks only the name and the founding year. `league_id` is writable, so a club's editor can promote it into any competition in the database.
- `RecordValue` is admin-only with the reasoning spelled out: *"otherwise every editor could inflate their own squad's valuations."* Competition membership is the same kind of editorial claim about standing, and it is left open.
- **Fix:** either require admin to change `league_id` (preserve the existing value otherwise, as `UpdatePlayer` does for `team_id`), or route it through `team_seasons` where the temporal model already says it belongs.
- File: [team_service.go:75-83](apps/football-svc/internal/service/team_service.go#L75-L83)

### N31. Smaller platform items
- [x] `WriteTimeout` is 30s but `ShutdownTimeout` is 15s, so a graceful drain can cut off requests that are still inside their own write budget. Shutdown should be the longer of the two.
- [x] `Recover` is the innermost middleware, so a panic in the tracing or metrics layer — which sit outside it — tears down the connection instead of returning a 500. The comment explains the ordering but not this consequence.
- [x] `DeleteTeam` does not call `Authorizer.InvalidateUser`, so cached grants for the deleted club survive up to the 5s TTL. Harmless today because the rows cascade away and the FK would reject a write, but it is the one mutation that skips the invalidation the other two perform.

---

# Round 2 resolution log — 2026-08-11

What was actually changed, per item.

## Toolchain (N1, N2, N29)

> **This was fixed the wrong way first. Corrected below — see N36.**

Aligned on **Go 1.25** for the three modules that require it, with CI, all
three Dockerfiles and the template raised to match, and golangci-lint moved to
v2.

Module directives are now whatever `go mod tidy` computes, which is the only
defensible source of truth: `apps/football-svc`, `apps/identity-svc` and
`libs/platform` are `1.25.0` because their dependency graphs demand it;
`libs/auth`, `libs/db` and `tests/integration` stay at `1.24.0` because theirs
do not. `go.work` is `1.25.0`, and CI's `GO_VERSION` is `1.25`, which satisfies
every module.

### N36. The first attempt at N1 aligned *down*, and broke CI
- [x] The original fix set every module to `go 1.24.0` on the reasoning that CI, the images and golangci-lint `v1.64.5` were already there, so aligning down touched four files instead of eleven and avoided a golangci-lint major upgrade.
- **That reasoning was wrong, and the evidence was available before making it.** The dependency graph *requires* 1.25: `github.com/felixge/httpsnoop@v1.1.0`, `github.com/nats-io/nats.go@v1.52.0` and `golang.org/x/crypto@v0.54.0` all declare `go >= 1.25`. 1.24 was never an option.
- Two failures followed in CI:
  - `go build ./...` failed outright with *"module github.com/felixge/httpsnoop@v1.1.0 requires go >= 1.25 (running go 1.24.13)"*. A **dependency's** requirement is a hard error; only the **main module's** directive triggers a toolchain switch, so nothing auto-upgraded.
  - The tidy check failed, because `go mod tidy` raises the directive back to `1.25.0` to satisfy those dependencies, and `git diff --exit-code` then sees the rewrite.
- **Why local verification missed it:** the local toolchain is Go 1.26.1, and with `GOTOOLCHAIN=auto` the build silently used it. `go build` going green on a machine with a newer toolchain says nothing about whether the pinned CI version can build the module. The check that would have caught it is reading what the dependencies require, not whether the build passes locally.
- **Fixed:** directives set by `go mod tidy` rather than by hand, `GO_VERSION` and all three Dockerfiles raised to 1.25, `apps/_template` raised to match, and the tidy fixpoint verified by snapshotting `go.mod`/`go.sum`, re-running tidy in all six modules and diffing — the check CI actually performs, which the first attempt never ran.

### N37. Turning the lint gate on surfaced 63 pre-existing violations
- [x] N2 established that golangci-lint had been rejecting three modules before linting them. Once it actually ran, it reported 63 issues. **These are not new — they are what the gate had been hiding.** Installed `golangci-lint v2.12.2` locally and worked through the whole list rather than the samples.
- Breakdown, and what each turned out to be:

| Linter | Count | Verdict |
|---|---|---|
| `noctx` | 48 | Test-only. Excluded. |
| `govet` (shadow) | 11 | All the standard idiom. Analyzer disabled. |
| `errcheck` | 2 | **Real.** Fixed. |
| `gosec` | 3 | False positives. Suppressed at the site. |
| `staticcheck` | 1 | **Real.** Fixed. |

- **`shadow` disabled, after reading all 11.** Every hit is `if err := f(); err != nil` or a block-scoped `x, err := ...`, in each case where the outer `err` had already been handled and returned on. One (`coach_spell_repository.go:98`) passes the outer `err` *into* the call that shadows it, which is correct by construction. The analyzer is experimental and off by default in `go vet` because it flags the idiom Effective Go recommends. Keeping it means renaming errors to `sizeErr`/`closeErr` at 11 sites — worse code, no defect caught. It was enabled in the round-1 config and its output had simply never been seen.
- **`noctx` excluded for `_test.go`**, joining `gosec`/`errcheck`/`unparam` which were already excluded there. It wants `httptest.NewRequestWithContext` and `Client.Do` over `Client.Get`. That matters in production, where an uncancellable request outlives the work that needed it; in a test served in-process and bounded by the test's own timeout it buys nothing at ~48 call sites. **No production code was excluded** — the two `noctx` hits in `libs/auth/jwks.go` were already fixed properly during N19.
- **Fixed for real:** `defer database.Close()` in both services now discards the error explicitly (`errcheck`), and a `weak.PublicKey.N` selector in a test I wrote for N6 was simplified to `weak.N` (`staticcheck QF1008`).
- **Suppressed with justification, not blanket rules:** `G101` on `refreshTokenColumns` matches the substring `token_hash` in a SQL column list; `G704` on `loadJWKSOnce` flags `JWKS_URL` as tainted, but fetching an operator-configured URL is the function's purpose — and anyone who can set `JWKS_URL` can already set `JWT_PUBLIC_KEY`. Each carries a `//nolint` naming the rule and the reason.

### N38. The lint timeout was too short for tests/integration
- [x] `tests/integration` reported `0 issues.` and then failed anyway with *"Timeout exceeded"* — it pulls in the testcontainers and Docker client trees, so type-checking alone exceeded the 5m budget. CI would have gone red on a module with nothing wrong with it.
- **Fixed:** `run.timeout` raised to 15m. Verified locally at exit code 0.

### N39. football-svc's integration test raced Postgres startup
- [x] `TestFootballServiceIntegration` failed in CI with `read tcp [::1]:...: connection reset by peer`, then reported *"Skipping test: testcontainers-go panicked (likely no Docker daemon)"* — on a runner where Docker was working perfectly.
- **Not caused by the round-2 fixes.** This test builds its own DSN with `sqlx.Connect` and never touches `libs/db`. It had almost certainly never run green: the integration job is gated on `needs: [build, migrations]`, and the build job was failing, so this was close to its first real execution.
- **Root cause:** the wait strategy was `wait.ForLog("database system is ready to accept connections")` with no occurrence count. The official Postgres image logs that line **twice** — first for the temporary server `initdb` runs to execute bootstrap scripts, which listens on a Unix socket only and is then **shut down**, and again when the real server starts on TCP. Waiting for the first means connecting while the server is restarting, which is exactly a connection reset.
- The identical suite in identity-svc already had `.WithOccurrence(2)`; its absence here was the only difference between one passing and one failing.
- **Fixed:** `wait.ForAll(ForLog(...).WithOccurrence(2), ForListeningPort("5432/tcp"))` with a 2-minute deadline. The port check covers the remaining gap, since the log line is written just before the socket is ready.

### N40. The failure was reported as three different wrong things
- [x] One connection error surfaced as a nil-pointer panic reported as a missing Docker daemon. Three separate defects stacked:
  - `assert.NoError` on the database connection let the test **continue** with a nil `*sqlx.DB` instead of stopping.
  - Several lines later that nil dereferenced and panicked.
  - The `defer recover()` block swallowed **every** panic and relabelled it "likely no Docker daemon".
- The net effect: a real, reproducible infrastructure race was presented as an environment problem, which is the kind of misreporting that gets a genuine failure dismissed as flake.
- **Fixed:** preconditions use `require` so a failure stops the test at the real error; the recover block is now bounded by a `containerStarted` flag and re-raises any panic that happens after the container is up. The same bounding was applied to identity-svc's copy, which had the same unbounded block.
- Also aligned: identity-svc's suite tested against `postgres:15-alpine` while compose and football-svc run 16.

### N41. Getting started required three tools Windows does not ship
- [x] The documented first run is `make keys && make up`, which needs **make**, **openssl** and **Docker**. On a clean Windows checkout none of the three is present, so the stack could not be started at all — and there was no documented alternative.
- Also: `smoke-test.sh`/`.ps1` validate the compose file and count running containers. Neither ever calls an endpoint, so "the stack is up" was the strongest claim available.
- **Fixed:**
  - `libs/auth/cmd/genkeys` generates the key pair in Go. `make keys` uses it, so openssl is no longer required anywhere.
  - `scripts/dev-setup.ps1` — creates the roles, databases and schema against an existing Postgres, with no Docker, make or openssl. Idempotent, and applies migrations in filename order against a local tracking table.
  - `scripts/dev-run.ps1` — runs either service from source with the right environment.
  - `scripts/check-endpoints.ps1` — walks the real API and asserts on responses: registration, login, RBAC, the full transfer flow, the temporal invariants, envelope shape, and the error contracts. This is the first thing in the repo that verifies the API rather than the deployment.
- The Docker path is unchanged and remains the reference; these are an alternative for machines without it.

### N42. The integration suite seeded a league that the schema rejects
- [x] CI: `pq: new row for relation "leagues" violates check constraint "leagues_competition_type_valid" (23514)`, seeding the league in `TestFootballServiceIntegration`.
- **Not a product defect — the API path is fine.** `POST /api/v1/leagues` goes through `validateLeague`, which defaults an unset `competition_type` to `league` before the row is written. A caller posting only a name and a country gets exactly what the documentation promises.
- **The test bypasses the service.** It seeds with `leagueRepo.Create` directly, so nothing fills the default in. The column has `NOT NULL DEFAULT 'league'`, but the repository writes the column explicitly, and an **explicit empty string overrides a DEFAULT rather than falling back to it** — so `''` reached the CHECK and was refused.
- **Why it surfaced only now**, three changes deep:
  - The integration job is gated on `needs: [build, migrations]`, and the build job had been failing, so this suite had not run in CI in a long time. The breakage was already there.
  - N22 mapped `23514` to `Invalid`, which turned an opaque 500 into an error naming the constraint.
  - N40 switched the seed from `assert` to `require`, so the run stopped at the real line instead of cascading into unrelated failures further down.
  - Together those three turned a silent, long-standing failure into a one-line diagnosis. That is the outcome they were for.
- **Fixed:** the seed sets `CompetitionType` explicitly, with a comment recording that this is only reachable from a direct repository call. Six service-level tests in `league_test.go` now pin the behaviour that actually matters — that an unset competition type is defaulted before the insert, never written as an empty string, and that every valid type round-trips.

### N43. `\gexec` never ran, so no role or database was created
- [x] `dev-setup.ps1` failed with `database "identity_db" does not exist`, after printing the `CREATE ROLE ...` and `CREATE DATABASE ...` statements as query results rather than executing them.
- **Two independent bugs, both of which produce garbage silently rather than failing:**

  **1. `\gexec` was given an empty query buffer.** `\gexec` executes the *current* buffer, so terminating the preceding `SELECT` with a semicolon sends and clears it first — leaving `\gexec` nothing to run. The generated statement is then merely displayed. The `SELECT` before a `\gexec` must not be semicolon-terminated.

  **This one also affected `deploy/postgres/init-databases.sh`**, which I rewrote in N24 to fix the password-escaping hole. The rewrite introduced this regression, which means **the Docker path would have failed to create its roles and databases** — the services would have started against a database that did not exist. CI never caught it: the migrations job uses its own Postgres service, and `docker compose config` only parses the file. The suite that would have caught it, `tests/integration`, has still never run.

  **2. PowerShell native-argument parsing.** `-v role=$svc.Role` expands only `$svc` and appends `.Role` as literal text, yielding `role=System.Collections.Hashtable.Role`. A *bare* `$svc.Db` token evaluates the property correctly, which is why `--dbname` worked while `-v role=` did not — the same script was half right, which is what made the failure confusing. Verified against a live shell rather than assumed.

- **Fixed:** semicolons removed before every `\gexec` in both files; all property access moved to locals, so no native-command argument interpolates a property. Both scripts now **verify** the database exists immediately after creating it and fail with a message naming the real cause, rather than letting a silently-skipped `\gexec` surface three steps later as a connection error.

### N44. Deleting a club failed once its players had an opening transfer
- [x] `DELETE /api/v1/teams/{id}` returned **400** instead of 204 for any club a player had joined. Found by `TestTeamDeletionNullsPlayerTeam`, the first time that suite ever ran to completion in CI.
- **A real product bug, not a test problem.** Two rules migration `000003` introduced contradict each other:
  - the foreign keys say *"a club may be deleted without erasing the transfer that references it"* — `from_team_id` and `to_team_id` are both `ON DELETE SET NULL`;
  - `transfers_has_direction` says *"a transfer must move a player from somewhere or to somewhere"* — `CHECK (from_team_id IS NOT NULL OR to_team_id IS NOT NULL)`.
- Both are reasonable; together they are impossible. A transfer naming exactly one club loses its only non-null reference when that club is deleted, and the CHECK then refuses the delete.
- **My N12 change is what made it reachable.** Creating a player now writes an opening transfer with `from_team_id` NULL and `to_team_id` set, so every club with players has such a row. Before that, a player created through the API had no transfers at all and the case never arose. N22 then surfaced the `23514` as a 400 rather than a 500, which is how it was identifiable at all.
- **Fixed** by migration `000006`, dropping the CHECK rather than the SET NULL behaviour. The two rules are enforced at different times and only one belongs in the database: *"needs a direction"* is a rule about **creating** a transfer, which `validateTransfer` already enforces with a clear message; *"history survives a club deletion"* is a rule about the row's whole lifetime, and a CHECK cannot tell an insert from a cascade. A transfer that has lost both clubs is degraded, not corrupt — player, date, type and fee all survive.
- The down migration re-adds the constraint `NOT VALID`, so a rollback succeeds even when such rows exist rather than requiring history to be deleted to satisfy it.
- `TestRecordTransfer_Validation`'s "no direction" case is now the sole enforcement, and says so in a comment.

### N45. The failing assertion was unreadable in CI
- [x] Four cycles were spent on N44 because the integration job emits a line per container per state change. The one `--- FAIL:` block was buried in hundreds of lines of compose output, and repeatedly scrolled past.
- **Fixed:** the job tees test output to a file and a `Failure summary` step, running `if: failure()`, greps the assertion, error trace and any diagnostic logging, printing it as the **last thing in the job**. `endpoint()` also now reports the container's published port map when a lookup fails, instead of a bare error.
- Worth doing on the first cycle, not the fourth: when a log buries its own signal, fix the reporting before continuing to read it.

### N2, revisited — golangci-lint v2
Aligning up forces the linter upgrade the first attempt was trying to dodge.
There is no golangci-lint v1 release that accepts a `go 1.25.0` module, so
`.golangci.yml` was migrated to the v2 schema (`version: "2"`,
`linters-settings` → `linters.settings`, `issues.exclude-rules` →
`linters.exclusions.rules`) and the workflow moved to
`golangci-lint-action@v8` with `version: v2.12.2`.

Both pins were checked against the GitHub releases API rather than assumed, and
the migrated config was validated against the published v2 JSON schema: every
key is accepted and all ten enabled linter names are valid.

## Security (N3–N9)

- **N3** — the Caddyfile now blocks `/api/identity/metrics` and
  `/api/football/metrics` explicitly, before the `handle_path` blocks that
  would otherwise strip the prefix and proxy them straight through.
- **N4** — a token-bucket limiter in `libs/platform/server/ratelimit.go`,
  applied per client to login (0.2/s, burst 5) and to register/refresh (1/s,
  burst 10). It lives in the service, not the gateway, because Caddy's
  rate-limit module is a third-party plugin absent from the official image —
  a control that only exists in a custom build is not a control. The Caddyfile
  comment claiming the gateway does rate limiting was corrected. Tests:
  `TestRateLimitBlocksAfterBurst`, `TestRateLimitIsPerClient`.
- **N5** — login now compares against a fixed dummy hash when no account
  matches, so both paths cost one bcrypt. The dummy is generated at startup
  from random bytes at `bcrypt.DefaultCost` rather than hardcoded, so it
  cannot drift from the real cost. Tests:
  `TestLogin_UnknownAccountStillHashes` (asserts a floor only the hash can
  clear, so it is not timing-flaky), `TestDummyPasswordHashMatchesRealCost`.
- **N6** — `checkKeySize` is now applied by `AddVerificationKey` and
  `JWK.toRSAPublicKey`, not just by `SetSigningKey`. `LoadJWKS` additionally
  requires `https` unless the host is loopback or an in-cluster name. Tests:
  `TestAddVerificationKey_RejectsWeakKey`, `TestJWKS_RejectsWeakKey`,
  `TestLoadJWKS_RequiresHTTPSForExternalHosts`.
- **N7** — see [Partial fixes](#partial-fixes).
- **N8** — Postgres, NATS and both service ports are bound to `127.0.0.1`.
- **N9** — `allowedOriginsFromEnv` returns nothing when unset instead of `["*"]`.
  The wildcard now has to be spelled out. The test that asserted the old
  fail-open default was rewritten to pin the new contract.

## Data integrity (N10–N17, N22)

- **N22** — `22P02` and `23514` added to `translate`. `/players/not-a-uuid`
  returns 400 instead of 500, and every schema `CHECK` now surfaces as a 400.
  Tests in `repository/errors_test.go`, including one asserting the driver's
  SQL fragment still never reaches the client.
- **N10/N11** — `MarketValueRepository.Delete` takes `(playerID, id)`, scopes
  the delete to both, and resets `players.market_value_minor` to the newest
  surviving valuation in the same transaction.
- **N12** — `PlayerRepository.Create` is now transactional and writes the
  opening transfer and opening valuation alongside the player, matching what
  migration `000003` backfilled for pre-existing rows.
- **N13** — `CoachSpellRepository.Record` establishes whether a later spell
  exists before doing anything, then refuses a backdated spell with no end
  date, and only updates `coaches.team_id` when the new spell is genuinely the
  latest.
- **N15** — `TransferRepository.Record` re-reads the player `FOR UPDATE` inside
  the transaction and rejects the write if the club moved underneath it; the
  squad update is guarded by `NOT EXISTS` on any later transfer, so backfilling
  history cannot rewrite the present.
- **N14/N17** — new migration `000005_constraint_gaps`: a `CHECK` on
  `coach_spells.role` (with `domain.SpellRole` and `ValidSpellRole` added, and
  the field retyped from `string` so the compiler enforces it), and an
  `EXCLUDE USING gist` constraint making overlapping seasons impossible.
  `SeasonRepository.Overlapping` gives the service layer a clear 409 instead of
  a raw constraint error.
- **N16** — see [Partial fixes](#partial-fixes).

## Platform (N18–N21, N23–N27, N30, N31)

- **N18** — `AuthMiddleware` and `server.Recover` emit the same JSON error
  shape as `httpx`. The shape is duplicated in `libs/auth` rather than
  imported, to keep that module free of a platform dependency. Bearer parsing
  also became case-insensitive per RFC 7235. Tests:
  `TestAuthMiddleware_ErrorsAreJSON`, `TestAuthMiddleware_SchemeIsCaseInsensitive`.
- **N19** — JWKS refetches on an unrecognised `kid` (rate-limited to once per
  30s so a bogus kid cannot drive a request per token) and refreshes on a
  5-minute ticker via `StartJWKSRefresh`, wired into football-svc.
  `TestValidateToken_RefetchesOnUnknownKeyID` simulates a real rotation by
  changing what the JWKS endpoint serves mid-test;
  `TestRefreshJWKS_IsRateLimited` pins the throttle.
- **N20** — `startSessionJanitor` runs `DeleteExpired` at startup and hourly.
- **N21** — empty `internal/domain/` removed; `DecodeJSON` rejects trailing
  content; `Minor.UnmarshalJSON` treats `null` as absent; the `000003` down
  migration restores the `coaches.team_id` UNIQUE constraint (releasing
  duplicate rows to NULL first, or it could not be re-added); `.env.example`
  documents `JWKS_URL`, `NATS_URL`, the pool settings and `DB_SSLMODE`; both
  services have healthchecks and the gateway waits on `service_healthy`.
- **N23/N24/N25** — `libs/db` rewritten: pool limits (25 open/25 idle, 30m
  lifetime) configurable by environment, DSN built through `net/url` so a
  password containing a quote or space cannot alter it, `DB_SSLMODE`
  configurable, `PingContext` with a 10s timeout, and `slog` instead of
  `log.Printf`. The same escaping class was fixed in `init-databases.sh`, which
  now passes identifiers and passwords through psql `:'var'` and `format()`
  `%I`/`%L` rather than shell interpolation into SQL run as superuser.
- **N26** — `SafePublisher` calls `RecordEventPublished`, so
  `events_published_total{outcome="error"}` is finally a real alarm. Alert on it.
- **N27** — an unreachable broker at startup now logs and falls back to
  `NopPublisher` instead of `log.Fatalf`. A NATS outage no longer takes down
  football-svc's read paths.
- **N30** — `UpdateTeam` preserves the stored `league_id` for non-admins.
  Authorization was also moved ahead of the database read, so an
  unauthenticated request no longer causes a query — a flaw introduced by the
  first version of this fix and caught by `TestTeamService_UnauthenticatedIsRejected`.
- **N31** — `ShutdownTimeout` is now `WriteTimeout + 5s`; `Recover` is
  installed both outermost (catching panics in metrics/tracing) and innermost
  (so application panics are still counted and traced); `DeleteTeam` calls the
  new `Authorizer.InvalidateTeam`.

## Partial fixes

Three items where the fix is narrower than the finding. Stating the remainder
plainly rather than marking them done and moving on.

### N7 — role-change lag
No code change. Implementing true immediate revocation means a shared
revoked-`jti` set consulted by every service on every request, which is new
cross-service state and a new failure mode on the hot path — too large to
attach to this batch, and 15 minutes is a defensible window.

**What was done instead:** the S2 entry, which claimed the old role "cannot
outlive the change", was corrected to say what is actually true. The gap was a
documentation error more than a code one.

### N16 — grants for users that may not exist
`Grant` now rejects a `user_id` that is not a well-formed UUID, which catches
the realistic mistake — a truncated or mistyped id written as a grant matching
nobody.

**Still open:** nothing verifies the account *exists* (that needs an
identity-svc call) and nothing cleans up grants when an account is deleted
(that needs a `user.deleted` event and a subscriber). The football service
owns no users, so neither can be solved locally. Worth doing when identity-svc
starts publishing events.

### N28 — at-most-once event delivery
No code change. A transactional outbox is the correct answer and is a
feature-sized piece of work.

**What was done instead:** the F6 entry now records the limitation explicitly,
so nobody builds a projection or read model on a bus that silently drops
messages. The `events_published_total` counter from N26 at least makes the
drops visible. Until an outbox exists, treat these events as advisory:
notifications and cache invalidation, not anything that must stay correct.

## Verification

```
gofmt -l .        clean across all six modules
go build ./...    all six modules
go vet ./...      all six modules
go test -short    all six modules pass
```

New tests: `repository/errors_test.go` (6), `libs/auth/hardening_test.go` (9),
`identity-svc/internal/handler/timing_test.go` (3), plus rate-limit and CORS
cases in `libs/platform/server/middleware_test.go`.

Two existing tests were changed rather than made to pass artificially:
`TestAllowedOriginsFromEnv` asserted the fail-open CORS default that N9 was
about, and `TestTeamService_UpdateTeam_RBAC` needed a `GetByID` expectation
once `UpdateTeam` began reading the stored league.

**Not verified here:** migration `000005` and the compose changes need Docker.
CI covers both — the migrations job applies, rolls back and re-applies every
migration, and validates the compose file.

---

# Round 2c — fallout from the round-2 fixes

Found by auditing the last unexamined files (the test suites, smoke scripts and
docs). Two are defects the round-2 fixes introduced. All fixed.

### N32. `db.Connect` reintroduced N23 for hand-built configs
- [x] The pool limits were applied from `LoadConfigFromEnv`, but `Connect` used whatever the `Config` carried. `database/sql` reads `MaxOpenConns == 0` as **unlimited**, so any caller constructing a `Config` literal — `tests/integration/stack_test.go` does exactly this, and so would any tool or script — silently got the unbounded pool N23 was about.
- **Fixed:** `Config.withDefaults()` fills the zero values inside `Connect`, so the limits hold regardless of how the config was built. Tests: `TestWithDefaults_ZeroConfigIsStillPooled`, plus `TestDSN_EscapesAwkwardPasswords` pinning the N24 escaping against passwords containing spaces, quotes and `sslmode=` injection attempts.

### N33. N12 and N15 together broke the stack test
- [x] `TestTransferHistoryIsRecorded` creates a player at a club, then records a transfer dated `2026-01-15`. With N12 that player now also gets an **opening transfer dated at creation time**, which is *later* than the move — so N15's backdating guard correctly refused to update the current club, and the history's newest row became the opening record (whose `from_team_id` is NULL). Two assertions would have failed.
- This only runs under Docker, so it passes locally and would have failed in CI.
- **Fixed in the fixture, not the guard.** The test now sets `contract_start`, which dates the opening transfer to when the player actually joined, making the 2026 move the later one. The guard is behaving correctly: a move that predates a player's own arrival record *is* contradictory data.
- Added `TestBackdatedTransferDoesNotRewriteCurrentClub`, which asserts the guard directly — a backdated move is recorded in the history but does not change where the player is today.

### N34. Documentation drift from the round-2 behaviour changes
- [x] The README's RBAC section said an editor "may write to clubs they hold a grant for" without noting the two exceptions (`league_id` after N30, market values already), and described the role-change endpoint in the same absolute terms N7 disproved.
- **Fixed:** both corrected, plus the at-most-once event caveat (N28) and the new rate limits recorded where an API consumer will see them.

### N35. Healthchecks outpaced the smoke scripts
- [x] Making the gateway wait on `service_healthy` (N21) lengthened startup, but `smoke-test.sh` counted running containers after 10s and `smoke-test.ps1` after 5. Both would have reported a false failure.
- **Fixed:** healthcheck `interval` and `start_period` tightened to 5s, and both scripts now wait 40s with a comment explaining the sequencing.

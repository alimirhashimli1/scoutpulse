# ScoutPulse Micro ⚽

A football database platform (Transfermarkt-style) built as a Go microservices
monorepo, with a shared platform library that keeps new services cheap to add.

## Project Structure

```text
/apps
  ├── football-svc    # Core domain: leagues, teams, players, coaches (Go)
  ├── identity-svc    # Auth & user management (Go)
  └── frontend        # UI dashboard (Angular — not started, Phase 4)
/libs
  ├── auth            # JWT issuance/validation, auth middleware, RBAC helpers
  ├── db              # Standardized PostgreSQL connection
  └── platform        # Shared service foundation:
      ├── apperr      #   error taxonomy (NotFound, Forbidden, Conflict, …)
      ├── server      #   HTTP bootstrap, middleware, graceful shutdown
      └── httpx       #   JSON encode/decode, error→status mapping, query params
/tests/integration    # Black-box tests against the full compose stack
```

Modules are tied together by `go.work`. Adding a service means one line there,
one in the Makefile's `MODULES`, and one in the CI matrix.

## Getting Started

```bash
cp .env.example .env     # then set JWT_SECRET to a random 32+ character value
make up                  # starts databases, runs migrations, starts services
make logs
```

| Service | URL |
|---|---|
| identity-svc | http://localhost:8080 |
| football-svc | http://localhost:8081 |
| identity Postgres | localhost:5433 |
| football Postgres | localhost:5434 |

Both services refuse to start without `JWT_SECRET`, and they must share the same
value: identity-svc signs tokens, football-svc verifies them.

### Common commands

```bash
make test       # unit tests, no Docker required
make test-all   # unit + integration tests (needs Docker)
make lint       # golangci-lint across every module
make tidy       # go mod tidy across every module
make migrate    # apply pending migrations to a running stack
make clean      # stop the stack and DELETE all database volumes
```

## Architecture

**Request flow:** `route → handler → service → repository → PostgreSQL`.
Dependencies are wired explicitly in each `main.go`; there is no DI framework.

**Layer responsibilities.** Handlers are pure transport: decode, delegate,
encode. All authorization lives in the service layer, so there is exactly one
place to read and to change it. Repositories own SQL and translate database
errors into the shared `apperr` taxonomy.

**Auth.** identity-svc issues an HS256 JWT carrying user id, role, and managed
team ids. `auth.AuthMiddleware` proves a caller holds a valid token; *which*
role may perform a write is decided by the service layer.

**RBAC.** `admin` has full access. `editor` may write to teams listed in their
token — including transferring a player where they manage either the current or
the destination club. `user` is read-only. Reads are public.

**Database per service.** Each service owns its own PostgreSQL database. Schema
changes are applied by golang-migrate jobs against a `schema_migrations` table,
so migrations are idempotent and safe to re-run against existing data.

## API

Reads are public; writes require `Authorization: Bearer <token>`.

### identity-svc

| Method | Endpoint | Notes |
|---|---|---|
| `GET` | `/health` | |
| `POST` | `/api/v1/auth/register` | `{username, email, password}` — always creates a `user`; roles are never client-assignable |
| `POST` | `/api/v1/auth/login` | `{identifier, password}` → `{token}`; identifier is email or username |

### football-svc

Full CRUD on `/api/v1/{leagues,teams,coaches,players}`. `GET /api/v1/teams`
requires `league_id`; `GET /api/v1/coaches` requires `team_id`.

**Paging.** List endpoints accept `limit` (default 25, max 100) and `offset`,
and return an envelope rather than a bare array:

```json
{ "items": [], "limit": 25, "offset": 0, "has_more": false }
```

**Filtering.** `GET /api/v1/players` accepts `free_agent`, `position`, `team_id`.

**Errors** are uniform, and never carry SQL or driver detail:

```json
{ "error": "team not found", "code": "not_found", "request_id": "…" }
```

Every response carries an `X-Request-ID`; an inbound one is preserved so a call
chain across services shares a single id in the logs.

## Status

- **Phase 1 Foundation** — complete
- **Phase 2 Identity Service** — complete
- **Phase 3 Football Service** — CRUD, RBAC, paging, and migrations complete;
  inter-service communication not started
- **Phase 4 Frontend** — not started

Known gaps and planned work are tracked in [ISSUES.md](ISSUES.md). The most
significant is **A7**: the domain model has no temporal dimension — no
transfers table, no market-value history, no seasons — which a Transfermarkt-style
product ultimately requires.

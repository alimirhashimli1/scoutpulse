# ScoutPulse Micro ⚽

A football database platform (Transfermarkt-style) built as a Go microservices
monorepo, with a shared platform library that keeps new services cheap to add.

## Project Structure

```text
/apps
  ├── football-svc    # Core domain: competitions, clubs, players, transfers (Go)
  ├── identity-svc    # Auth, users, token issuance (Go)
  ├── _template       # Skeleton for new services — see make new-service
  └── frontend        # UI dashboard (Angular — not started, Phase 4)
/libs
  ├── auth            # RS256 JWT issuance/verification, JWKS, middleware
  ├── db              # PostgreSQL connection
  └── platform        # Shared service foundation:
      ├── apperr        #   error taxonomy (NotFound, Forbidden, Conflict, …)
      ├── server        #   HTTP bootstrap, middleware, graceful shutdown
      ├── httpx         #   JSON, error→status mapping, query params
      ├── events        #   NATS publish/subscribe and the event contract
      └── observability #   Prometheus metrics, OpenTelemetry tracing
/deploy
  ├── gateway         # Caddy — the single entry point
  ├── postgres        # Database and role bootstrap
  └── prometheus      # Scrape configuration
/tests/integration    # Black-box tests against the full compose stack
```

Modules are tied together by `go.work`. `make new-service` scaffolds a new one.

## Getting Started

```bash
cp .env.example .env
make keys                # generates the JWT key pair into .env — run once
make up                  # databases, migrations, NATS, services, gateway
make logs
```

Everything is reachable through the gateway:

| What | URL |
|---|---|
| **Gateway** | http://localhost:8000 |
| identity via gateway | http://localhost:8000/api/identity/api/v1/... |
| football via gateway | http://localhost:8000/api/football/api/v1/... |
| JWKS | http://localhost:8000/.well-known/jwks.json |

Service ports (`8080`, `8081`) and Postgres (`5432`) stay exposed for local
debugging, but application traffic should go through the gateway — adding a
service is then a route block, not a new port for clients to learn.

```bash
make observability       # Prometheus :9090, Jaeger :16686
```

### Common commands

```bash
make test        # unit tests, no Docker required
make test-all    # unit + integration tests (needs Docker)
make lint        # golangci-lint across every module
make fmt         # gofmt across every module
make migrate     # apply pending migrations to a running stack
make new-service NAME=transfer-feed PORT=8082
make clean       # stop the stack and DELETE all database volumes
```

## Architecture

**Request flow:** `gateway → route → handler → service → repository → PostgreSQL`.
Dependencies are wired explicitly in each `main.go`; there is no DI framework.

**Layer responsibilities.** Handlers decode, delegate, encode. All authorization
lives in the service layer, so there is one place to read and change it.
Repositories own SQL and translate database errors into the shared taxonomy.

**Auth.** identity-svc holds the RSA private key and is the only service that can
mint a token; everyone else verifies with the public key from `/.well-known/jwks.json`.
Access tokens last 15 minutes; a rotating, revocable refresh token carries
longer sessions.

**RBAC.** `admin` has full access. `editor` may write to clubs they hold a grant
for — including recording a transfer where they manage *either* side of it, but
**not** changing a club's `league_id` or setting market values, both of which
are claims about standing that an editor should not be able to make about their
own club. `user` is read-only. Reads are public. Grants live in the football
service's database and are resolved per request, so granting and revoking are
immediate.

Changing a user's *role*, by contrast, takes effect within one access-token
lifetime (15 minutes): it ends their refresh tokens at once, but an access token
already issued carries the old role until it expires.

**Events are at-most-once.** A subscriber can drive notifications or cache
invalidation; it cannot drive a projection that must stay correct, because a
broker outage drops events silently. See N28 in `ISSUES.md`.

**Data ownership.** One Postgres instance, one database and login role per
service. Schema changes are applied by golang-migrate against a
`schema_migrations` table, so migrations are idempotent and safe on existing data.

**Events.** football-svc publishes domain facts to NATS. A new app subscribes
rather than the football service being changed to call it — which is what lets
you add apps without touching the core.

## The domain model

Current state is **derived** from history, never the other way round:

| Source of truth | Derived from it |
|---|---|
| `transfers` | `players.team_id` (current club) |
| `player_market_values` | `players.market_value_minor` (latest value) |
| `coach_spells` | `coaches.team_id` (current appointment) |

A transfer writes the history row and moves the player in one transaction, so
the squad can never disagree with the record. Derived columns exist so the
common reads stay a single-table lookup.

`seasons` and `team_seasons` supply the temporal frame: a club's competition, a
squad, and a transfer are all statements about a particular season.

## API

Reads are public; writes require `Authorization: Bearer <token>`.

### identity-svc

| Method | Endpoint | Notes |
|---|---|---|
| `GET` | `/health` | |
| `GET` | `/.well-known/jwks.json` | Public keys, for verifiers |
| `POST` | `/api/v1/auth/register` | `{username, email, password}` — always creates a `user`; roles are never client-assignable |
| `POST` | `/api/v1/auth/login` | `{identifier, password}` → token pair |
| `POST` | `/api/v1/auth/refresh` | `{refresh_token}` → new pair; the refresh token rotates |
| `POST` | `/api/v1/auth/logout` | Revokes a refresh token |
| `GET` | `/api/v1/users/me` | The authenticated account |
| `PUT` | `/api/v1/users/{id}/role` | Admin only; ends the user's refresh tokens (see the RBAC note on timing) |

`register`, `login` and `refresh` are rate limited per client IP — login most
tightly, since every attempt costs a bcrypt hash whether or not the account
exists. Exceeding it returns `429` with a `Retry-After` header.

Login and refresh return:

```json
{ "access_token": "…", "refresh_token": "…", "token_type": "Bearer", "expires_in": 900 }
```

### football-svc

CRUD on `/api/v1/{leagues,seasons,teams,coaches,players,transfers}`, plus:

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/transfers` | The transfer feed. Filters: `player_id`, `team_id`, `season_id`, `type`, `min_fee_minor` |
| `GET /api/v1/players/{id}/transfers` | One player's career |
| `GET /api/v1/players/{id}/market-values` | Valuation history |
| `GET /api/v1/coaches/{id}/spells` | A coach's career |
| `GET /api/v1/teams/{id}/coaches` | A club's managerial history |
| `GET /api/v1/seasons/current` | The season containing today |
| `POST /api/v1/teams/{id}/editors` | Grant edit access (admin) |

`GET /api/v1/teams` requires `league_id`; `GET /api/v1/coaches` requires `team_id`.
`GET /api/v1/players` filters on `free_agent`, `position`, `team_id`,
`nationality`, `min_value_minor`, `max_value_minor`.

**Paging.** Lists accept `limit` (default 25, max 100) and `offset`, and return
an envelope:

```json
{ "items": [], "limit": 25, "offset": 0, "has_more": false }
```

**Money** is always an integer count of minor units (cents), never a decimal —
binary floating point cannot represent decimal currency exactly.

**Errors** are uniform and never carry SQL or driver detail:

```json
{ "error": "team not found", "code": "not_found", "request_id": "…" }
```

Every response carries `X-Request-ID`; an inbound one is preserved, so a call
chain across services shares a single id in the logs and traces.

## Status

- **Phase 1 Foundation** — complete
- **Phase 2 Identity Service** — complete
- **Phase 3 Football Service** — complete: full CRUD, temporal domain model,
  service-layer RBAC, paging, migrations, events
- **Phase 3.5 Platform** — complete: workspace, shared platform library,
  gateway, event bus, observability, service template
- **Phase 4 Frontend** — not started. The API is stable to build against now
  that the temporal model is in place.

Open items are tracked in [ISSUES.md](ISSUES.md).

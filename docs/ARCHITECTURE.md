# ScoutPulse — how I built it

A walkthrough of the whole system, written the way I would explain it in an
interview: what it is, how it fits together, the order I built it in, the tools
I used, and the decisions (and mistakes) behind each part.

---

## 1. The 30-second version

ScoutPulse is a football database in the style of Transfermarkt: competitions,
clubs, squads, player profiles, transfer histories, market values and season
statistics. On top of the database it has two creative tools: an animated **3D
tactics board** and an **editorial platform** for analysis articles.

Technically it is:

- **Two Go microservices**: `identity-svc` (accounts, login, tokens) and
  `football-svc` (all the football data, tactics boards and articles).
- **Three shared Go libraries**, for auth, the database connection and a
  "platform" layer with the HTTP server, errors, events and observability.
- **An Angular 21 frontend**, server-side rendered for SEO.
- **PostgreSQL**, with one database per service.
- **Deployed** with the frontend on Vercel, the Go services on Railway and the
  databases on Supabase. Locally, the whole stack runs in Docker Compose behind
  a Caddy gateway.

In numbers: about 22,000 lines of Go and 24,000 lines of TypeScript, 91 HTTP
endpoints (70 football, 21 identity), 15 database migrations, more than 300
automated tests, and 90 commits between May and October 2026.

---

## 2. The big picture

```
                          Browser
                             │
             ┌───────────────┴────────────────┐
             │  Vercel  (production edge)     │   locally: Caddy gateway :8000
             │  /api/football/* ─────────┐    │   (same path-prefix routing)
             │  /api/identity/* ───────┐ │    │
             │  /.well-known/jwks.json │ │    │
             │  /*  → Angular SSR fn   │ │    │
             └─────────────────────────┼─┼────┘
                                       │ │
                 ┌─────────────────────┘ └──────────────────┐
                 ▼                                          ▼
        identity-svc (Go, :8080)                   football-svc (Go, :8081)
        signs JWTs (RSA private key)               verifies JWTs (public key via JWKS)
        users, refresh tokens, OAuth,              leagues, clubs, players, transfers,
        email verification, captcha                values, stats, notes, tactics, articles
                 │                                          │
                 ▼            ── NATS (optional) ──▶        ▼
          identity_db          user.deleted,          football_db
          (Postgres)           user.role_changed      (Postgres)
```

The request flow inside each service is always the same:

```
HTTP → middleware chain → handler → service → repository → PostgreSQL
        (request id, logging,   decode/   rules &   SQL & error
         CORS, metrics,         encode    authz     translation
         tracing, recovery)
```

---

## 3. Tools and technologies, and why I chose each one

| Area | Tool | Why |
|---|---|---|
| Backend language | **Go 1.25** | Fast, simple concurrency, a single static binary per service, and a standard library good enough that I needed almost no framework. |
| HTTP routing | **`net/http` ServeMux** (Go 1.22+ patterns such as `GET /players/{id}`) | Method and path-parameter routing are built in now, so I didn't need chi or gin. |
| SQL | **sqlx** + **lib/pq** | Hand-written SQL with struct scanning. I wanted to control every query, especially the transactional ones, rather than use an ORM. |
| Migrations | **golang-migrate** | Versioned up/down SQL files tracked in a `schema_migrations` table. |
| Auth | **golang-jwt/jwt v5**, RS256, JWKS | Asymmetric signing, so only the identity service can create tokens (details in §6). |
| Passwords | **bcrypt** | The standard choice: slow hashing with a built-in salt. |
| Events | **NATS** (JetStream enabled) | A light message bus, so services react to each other without calling each other directly. |
| Observability | **Prometheus** metrics, **OpenTelemetry** tracing → **Jaeger**, **slog** JSON logs | Metrics, traces and structured logs, all linked by a request id. |
| Gateway | **Caddy** | One public origin that routes by path prefix, in a 30-line config. |
| Frontend | **Angular 21**: standalone components, **signals**, zoneless, **SSR** with Express 5 | Signals make state simple to follow, and SSR is needed for SEO on public pages. |
| 3D | **Three.js** r169 | WebGL rendering for the tactics board. |
| Rich text | **Tiptap 3** (ProseMirror) | A structured document model instead of raw HTML, so content can be validated on the server. |
| Frontend tests | **Vitest** + jsdom | Fast. I switched it to the `threads` pool after `forks` hung on Windows. |
| Backend tests | Go `testing`, **testify**, **testcontainers-go** | Unit tests with fakes, plus integration tests against a real Postgres started in Docker. |
| Lint and format | **golangci-lint v2**, gofmt, **Prettier** | Enforced in CI. |
| Containers | **Docker** multi-stage builds, **Docker Compose** | The same images run locally and in CI. |
| CI | **GitHub Actions** | Build, lint and test matrix; migration round-trip; frontend SSR checks. |
| Hosting | **Vercel** (frontend), **Railway** (Go services), **Supabase** (Postgres) | Managed services with free tiers that fit each part. |
| Scripting | **Make**, **PowerShell** scripts | `make up/test/lint/keys/new-service`; Windows scripts for setup, seeding, backup and restore. |
| AI assistance | **Claude Code** (plus Vercel MCP) | Used as a pair programmer for implementation, reviews and audits. Every change was built, tested and reviewed before merging. |

---

## 4. How I built it, step by step

I built it in phases, each one leaving a working, tested system.

### Phase 1: Foundation (May 2026)

- Set up a **monorepo** with a **Go workspace** (`go.work`) that ties together
  six modules: two apps, three libraries and the integration-test module.
- Wrote the first schema: leagues, teams, coaches, players.
- Started with the simplest thing that worked: a direct database call inside
  each handler. That didn't stay for long.

### Phase 2: Identity service

- Registration and login with bcrypt.
- Refactored to the **repository pattern**: handlers stopped touching SQL, and
  the database layer moved behind interfaces, so tests can swap in fakes.
- Extracted a **shared auth library** (`libs/auth`) so every service verifies
  tokens and checks roles the same way.
- Wrote a **multi-stage Dockerfile** and the first **integration test** with
  testcontainers-go: start a real Postgres, create a user, log in, receive a
  valid JWT.

### Phase 3: Football service

- Full CRUD for every entity, with paging.
- **The most important design change**, migration `000003`, introduced a
  **temporal domain model** (§5.3). The first schema just overwrote a player's
  club, so history was lost. Now transfers, market values and coaching spells
  are the source of truth, and the "current club" is derived from them.
- **Role-based access control** in the service layer, with per-club editor
  grants (§5.4).

### Phase 3.5: Platform

Once there were two services, I extracted what both needed into
`libs/platform`, so a third service costs very little:

- `server`: hardened timeouts, a standard middleware chain and graceful
  shutdown.
- `apperr`: an error taxonomy. `httpx`: JSON, error-to-status mapping and query
  parsing.
- `events`: NATS publish/subscribe and the event contracts.
- `observability`: Prometheus and OpenTelemetry.
- A **service template** plus `make new-service NAME=x PORT=y`, which
  scaffolds a new service.
- The **Caddy gateway**, so the frontend only ever talks to one origin.

### Audits

Before starting the frontend, I audited the backend in rounds and kept every
finding in `ISSUES.md`: 82 issues found, 81 fixed, sorted by priority (P0
blocking, P1 security, and so on). Examples include a Slowloris hole (no
`ReadHeaderTimeout`), refresh tokens that never expired from the table (no
cleanup job), editor permissions frozen inside tokens, and ports published on
all network interfaces.

### Phase 4: Frontend

I wrote the design in `docs/FRONTEND.md` first, then built it in phases:

0. Scaffold with SSR, zoneless change detection and design tokens.
1. Core plumbing: wire types, `Page<T>`, `ApiError`, repositories,
   interceptors.
2. Auth: the session store, a single-flight refresh, login and register pages,
   OAuth callback.
3. The public pages: transfer feed, player, club, competition, coach, season
   and search pages.
4. Write forms and admin pages.

### Phase 5: Product features

- **Full-text search** across players, clubs, coaches and competitions.
- **Player profile depth and member notes**: one note per member per player.
- **Tactics board**: a 3D animated board with an editor, shareable links,
  video export and, most recently, a 2D mode.
- **Editorial platform**: an article editor with a review workflow,
  scheduling, and embedded tactics boards and videos.
- **Security hardening**: the refresh token moved to an HttpOnly cookie, a
  strict Content-Security-Policy, and tighter rate limits.

### Phase 6: Production and real data

- Deployed to Vercel, Railway and Supabase (§9).
- Built a **data importer** that loads the open Transfermarkt dataset for the
  top 10 European leagues: about 240 clubs, 6,600 players, 22,000 transfers,
  100,000 market-value points and 25,000 season stat lines (§8.4).

---

## 5. Backend deep dive

### 5.1 Monorepo and modules

```
apps/identity-svc     apps/football-svc     apps/_template
libs/auth             libs/db               libs/platform
tests/integration
```

- `go.work` lets local development use the libraries straight from disk.
- Each Docker build sets `GOWORK=off` and builds from the service's own
  `go.mod` (with `replace` directives pointing at `libs/`). That keeps images
  reproducible, but it means the build context must be the repository root.
- Dependencies are **wired by hand in `main.go`**, with no DI framework:
  create the repositories, then the services, then the handlers, then register
  the routes. It's explicit, easy to read, and a missing dependency is a
  compile error.

### 5.2 The layers

- **Handler**: decodes the request, calls one service method, encodes the
  response. No business rules.
- **Service**: validation, business rules and **all authorization**. That way
  there is exactly one place to read who may do what.
- **Repository**: owns the SQL and translates database errors (unique
  violation → `Conflict`, no rows → `NotFound`) into the shared `apperr`
  taxonomy.
- **`httpx.WriteError`** maps the error kind to an HTTP status and always
  returns the same shape, never exposing SQL or driver details:

  ```json
  { "error": "team not found", "code": "not_found", "request_id": "…" }
  ```

### 5.3 The domain model

**Current state is derived from history, never the other way round:**

| Source of truth | Derived from it |
|---|---|
| `transfers` | `players.team_id` (current club) |
| `player_market_values` | `players.market_value_minor` (latest value) |
| `coach_spells` | `coaches.team_id` (current job) |

- Recording a transfer writes the history row **and** moves the player in one
  transaction, with the player row locked (`SELECT … FOR UPDATE`). A squad can
  therefore never disagree with the transfer record.
- That's also why `PUT /players/{id}` ignores `team_id`: the only way to move a
  player is to record a transfer.
- The derived columns exist so that common reads stay single-table lookups.

Other modelling decisions:

- **Money is integer minor units** (cents) in a `BIGINT`. Floating point can't
  represent decimal currency exactly, and the API rejects decimal input rather
  than rounding it.
- **Competitions** are one table with a type (league, domestic cup,
  international cup, super cup), because a club plays several competitions in
  one season.
- **`seasons` and `team_seasons`** give every fact a time frame: which
  competition a club played in, and in which season.
- **Constraints in the database**, not just in code: `CHECK` constraints for
  date order, valid enum values, non-negative numbers and article status rules.
  The database is the last line of defence.

### 5.4 Authorization (RBAC with grants)

- Three roles: `admin`, `editor` and `user`. **Reads are public.**
- An `editor` may only write to clubs they hold a **grant** for (the
  `team_editors` table). A transfer can be recorded by an editor of *either*
  club involved.
- Editors may **not** change a club's league or set market values. Those are
  claims about a club's standing, which an editor shouldn't be able to make
  about their own club.
- Grants are **not stored in the JWT**. I tried that first, and it froze
  permissions for the token's lifetime: a revoked grant kept working until the
  token expired. Now the `Authorizer` looks grants up per request, with a small
  cache that is cleared when a grant changes.
- The routes do two things only: `AuthMiddleware` proves *who you are*, and
  the service layer decides *what you may do*.

### 5.5 Search

Migration `000007` adds:

- A **weighted `tsvector`** column on players (name A, first and last name B,
  nationality and position C), with a **GIN index**, for full-text matching.
- A **`pg_trgm` trigram index** on names, for typo-tolerant prefix matching.

One `/search?q=&kind=` endpoint ranks results across all entity types.

### 5.6 Migrations

- There are 11 football migrations and 4 identity migrations. All are
  **idempotent** (`IF NOT EXISTS`) and all have a working `down` file.
- **CI applies every migration up, then down, then up again** against a real
  Postgres. A migration that can't be rolled back and re-applied isn't safe to
  deploy.

### 5.7 Rate limiting

- I wrote an in-process **token-bucket** limiter in `libs/platform/server`,
  with a sweeper that removes idle buckets.
- `login` gets a burst of 5 and then 0.2 requests per second per IP. There is
  **also** a per-account failure limit: 10 failures, then one attempt every 3
  minutes. Attackers can rotate IP addresses, but not the account they're
  attacking.
- Every authenticated write in football-svc is limited **per account** (a
  burst of 60, then 2 per second), because accounts are free to create.
- The client IP header is configurable (`CLIENT_IP_HEADER`), since the trusted
  header depends on which proxy sits in front: Caddy or Railway.

### 5.8 Events

- football-svc publishes domain facts such as `PlayerTransferred`,
  `PlayerValueChanged` and `TeamCreated`.
- identity-svc publishes `UserDeleted` and `UserRoleChanged`. football-svc
  **subscribes** to these and removes the user's editor grants, boards and
  drafts. That data lives in a different database, so no foreign key could
  enforce it.
- **NATS is optional.** Without `NATS_URL` the publisher does nothing and the
  service still runs, because an event is a side effect of a write, not part of
  it. A `SafePublisher` logs and counts failures but never fails the request.
- I documented the trade-off: delivery is **at most once**. A subscriber may
  drive notifications or cache invalidation, but it must not maintain a
  projection that has to stay correct.

### 5.9 Observability

- Every request gets an **`X-Request-ID`**, or keeps the one it arrived with,
  so a chain of calls across services shares a single id. It appears in logs,
  traces and error bodies, so a user can quote it.
- **slog** writes JSON logs, one line per request. Successful health checks and
  metrics scrapes are skipped, because thousands of identical lines bury the
  real ones.
- **Prometheus** collects request counters and latency histograms, labelled by
  *route pattern* rather than raw path, so a URL like `/players/{id}` doesn't
  create a new label for every id. `/metrics` is protected by a token.
- **OpenTelemetry** traces go to Jaeger, with sampling. If no collector is
  configured, tracing does nothing: telemetry must never be a dependency of
  serving a request.

### 5.10 Server hardening

- `ReadHeaderTimeout` closes the Slowloris hole that a plain
  `http.ListenAndServe` leaves open. There are also read, write and idle
  timeouts.
- **Graceful shutdown**: on SIGTERM the server stops accepting connections and
  lets in-flight requests finish. The shutdown window is deliberately longer
  than the write timeout.
- Containers run as a **non-root user** on Alpine, with stripped static
  binaries (`CGO_ENABLED=0`, `-ldflags="-s -w"`).

---

## 6. Identity and security deep dive

### 6.1 Tokens

- **RS256, asymmetric.** identity-svc holds the RSA **private** key and is the
  only service that can mint a token. Every other service fetches the
  **public** key from `/.well-known/jwks.json` and refreshes it periodically,
  so a key rotation reaches them without a redeploy.
  - I started with a shared HMAC secret. Under that design, any service able
    to *verify* a token could also *forge* an admin token, which stops being
    acceptable once there is a third service.
- The algorithm is **pinned** to RS256 during validation, to prevent
  algorithm-confusion attacks.
- **Access token**: 15 minutes, held in **memory** only.
- **Refresh token**: 30 days, **stored server-side as a hash**, **rotated on
  every use**, and sent to browsers in an **HttpOnly, SameSite=Strict
  cookie**, so no script can read it.
- **Reuse detection**: if a refresh token that was already exchanged comes back,
  it's treated as stolen and **every session for that user is revoked**.
- A password or role change also ends all of the user's refresh tokens.

### 6.2 The single-flight refresh, the most important piece of frontend code

Reuse detection creates a trap. When an access token expires, a page often
fires three requests at once, and all three get a 401. If each one triggered
its own refresh, the first would succeed, the other two would present a token
that was already used, and the user would be **logged out everywhere**.

So the HTTP interceptor holds **one in-flight refresh promise at module
scope**. The first 401 starts the refresh, every other request waits on the
same promise, and all of them retry with the new token. Ten unit tests cover
this, including the concurrent case.

### 6.3 Other identity features

- **OAuth with Google and Facebook**, using a redirect flow.
  - The backend completes the provider callback, then sends the browser to
    `/auth/callback?code=…`.
  - That code is **single-use and expires in 60 seconds**; the frontend
    exchanges it for tokens. Tokens therefore never appear in a URL.
- **Email verification**: tokens are stored hashed. Without SMTP configured,
  the link is written to the log, so the flow can be tested without a mail
  account.
- **Captcha** (reCAPTCHA or Turnstile) is **verified on the server**. The
  widget alone proves nothing, since a bot can skip it and call the API
  directly.
- **No account enumeration**: login costs one bcrypt hash whether or not the
  account exists, and a timing test checks this.
- **Cleanup jobs**: background goroutines delete expired refresh tokens hourly
  and expired login codes every 15 minutes.
- **Roles are never assigned by the client**: registration always creates a
  `user`.

### 6.4 Security summary

| Threat | Defence |
|---|---|
| Forged tokens | RS256 with only the issuer holding the private key; algorithm pinned |
| Stolen refresh token | HttpOnly cookie, rotation, reuse detection revoking all sessions |
| XSS | Strict CSP with hashed inline scripts; validated article schema; access token in memory only |
| CSRF | SameSite=Strict cookie, plus a custom header that a cross-origin page can't send without a preflight |
| Brute force | Per-IP and per-account rate limits; captcha |
| SSRF via the SSR Host header | `NG_ALLOWED_HOSTS` allow-list |
| Clickjacking | `frame-ancestors` and `X-Frame-Options` |
| SQL injection | Parameterised queries only |
| Exposed infrastructure | Postgres, NATS and service ports bound to loopback in Compose |

---

## 7. Frontend deep dive

### 7.1 Angular setup

- **Angular 21** with standalone components only: no NgModules.
- **Zoneless change detection** with **signals**. State is held in `signal()`,
  derived values use `computed()`, and side effects use `effect()`. No
  zone.js.
- `withComponentInputBinding()`, so route parameters arrive as component
  inputs.
- **Every route is lazy-loaded** with `loadComponent`, so the first download
  contains only the app shell.
- **Vanilla CSS** with design tokens (`--accent`, `--space-*`, `--text-*`),
  light and dark themes, and per-component style budgets enforced by the
  build.

### 7.2 Architecture, applying SOLID concretely

```
core/       singletons: api, auth, models, tokens, seo, theme
shared/     reusable UI, pipes, forms, pagination — no feature knowledge
features/   one folder per area, lazy-loaded
layout/     shell, header, footer
```

- **Dependencies only point inwards.** `features` may import from `shared` and
  `core`, and `core` imports from neither. Features never import from each
  other.
- **Interface segregation and dependency inversion**: each resource has
  separate **reader** and **writer** interfaces, each behind an
  `InjectionToken`. The public player page injects only `PLAYER_READER`, so
  the *compiler* guarantees it can't write. One HTTP class implements both,
  bound with `useExisting`.
- **No component injects `HttpClient`.** Data access goes through
  repositories, so tests swap in a provider instead of mocking HTTP.
- **Typed errors**: an `errorInterceptor` turns every failure into an
  `ApiError` with a typed `code`, so error handling happens once, centrally.
- **`Page<T>`** mirrors the API's paging envelope. The paginator shows
  next/previous plus the range on screen, because the API reports `has_more`
  instead of a total (counting every matching row on each request is
  expensive).

### 7.3 Server-side rendering: the hardest part of the frontend

- **Hybrid rendering per route** (`app.routes.server.ts`):
  - Public pages (players, clubs, transfers…) are **rendered on the server per
    request**. They need SEO, and their ids live in a database, so they can't
    be prerendered at build time.
  - Forms, the account page and admin pages render **client-side only**. The
    server has no session, so guards can't be evaluated there and nothing on
    those pages is worth indexing.
- **Every public route is listed explicitly**, so the catch-all route returns a
  real **404** rather than a "soft 404" (a 200 page that says "not found"). CI
  checks this.
- **`ssrResource`**: Angular 21 doesn't transfer `resource()` state from the
  server to the browser, so the browser started with an empty value, rendered
  a loading skeleton, and caused a hydration mismatch. My wrapper stores the
  server's first resolved value in `TransferState` and seeds the browser with
  it, so the first client render matches the server's HTML.
- **The Host-header trap**: Angular 21 validates the `Host` header (to prevent
  SSRF) and, if it fails, **silently falls back to client rendering**. I lost
  time to "SSR does nothing" before finding this. The fix is
  `NG_ALLOWED_HOSTS`, plus `TRUST_PROXY_HEADERS` behind a proxy.
- **CSP with SSR**: Angular's `autoCsp` doesn't work with SSR, so `server.ts`
  builds the policy itself. It hashes the template's inline scripts once at
  startup, and hashes the event-replay script per response, but only if that
  script matches its exact expected shape.
- **SEO**: a `Seo` service sets the title, description, canonical URL and Open
  Graph tags for every page. Search results pages are `noindex, follow`.

### 7.4 Other frontend details

- **`LookupStore`**: the API returns ids, not embedded objects. Rather than
  make N+1 requests, clubs, competitions and seasons are each loaded once and
  cached.
  - I documented its limit: it won't scale past a few thousand clubs. That
    became real once the importer brought in 240 clubs, so I scoped the import
    to keep the list small. The next step is a batch `?ids=` or `?expand=`
    endpoint.
- **The market-value chart is hand-drawn SVG**, with no chart library. One
  sparkline doesn't justify a dependency, and SVG renders the same way on the
  server.
- **`MoneyPipe`** formats minor units, including compact forms such as "€85m"
  and "undisclosed" vs "free" (16 tests).
- **Session restore**: on page load in the browser, the HttpOnly cookie is
  exchanged for a new access token before the first route renders. On the
  server, the session is simply anonymous.
- **Guards are for navigation only**. The API enforces every rule
  independently; a guard just avoids showing a page that would return 403.

---

## 8. Feature deep dives

### 8.1 Tactics board

**What it does:** a coach places players and equipment, draws arrows and zones,
and builds a drill step by step. Players watch it animate in 3D or 2D, through
a share link that needs no account. It can also be exported as a video.

**The data model** (`board/model.ts`):

- A board is **stored as data, not as video**: objects, their position at each
  step, and drawings. A whole drill is tens of kilobytes of JSON in one
  `tactic_boards` row, and it stays editable after it has been shared.
- **Coordinates are metres** on a 105 × 68 pitch, with the origin at the centre
  spot. A board then looks the same on a phone and on a projector, and a
  20-metre rondo grid really is 20 of something.
- Documents are **versioned**, and `normalizeDoc` is the single place where an
  older shape is upgraded.

**The architecture:**

- **`BoardEngine`** is a plain TypeScript class with **no Angular in it**. It
  owns the WebGL canvas, draws whatever frame it's given, and reports pointer
  gestures in pitch coordinates. *What a gesture means* (add a player, move one
  in this step) is decided by the editor store, so the same engine serves the
  read-only viewer unchanged.
- **`playback.ts`** is pure functions: a document becomes a timeline, and a
  time becomes a frame. Movement uses easing, bent paths follow quadratic
  curves, and a ball pass can fly through the air. All of it is unit-tested
  without WebGL.
- **`BoardPlayer`** holds the clock, built from signals. It advances once per
  rendered animation frame, so the picture never runs ahead of or behind the
  time shown.
- **Three.js is loaded with a dynamic `import()`** after the first render. It
  never loads on the server (which has no WebGL), and only pages that show a
  board download it.
- **2D mode**: the same engine with an **orthographic camera** looking straight
  down. Players are drawn as flat counters with their numbers, there are no
  stands or shadows, and the controls pan and zoom without rotating.
- **Video export** uses `canvas.captureStream()` with **`MediaRecorder`**,
  entirely in the browser: MP4 where supported, WebM otherwise. Nothing is
  uploaded.
- **Editor**: undo/redo, steps that can be duplicated and reordered by
  dragging, per-step captions, team kits and formations. A signed-out visitor
  can build a board, and their draft is kept through sign-in.
- **Boards are unlisted**: anyone with the link can view one, but the list of
  boards is private to its owner.

### 8.2 Editorial platform (analysis articles)

- **The editor is Tiptap (ProseMirror)**, loaded only on the editor page. The
  body is a **JSON document tree**, not HTML.
- **The schema is enforced on the server** (`article_body.go`). Every node type
  must be one the reader knows how to draw, and every URL (links, images,
  embedded boards and videos) is checked. The reader checks again as it
  renders. That is defence in depth against stored XSS.
- **Workflow**: `draft → in_review → changes_requested → published`. Writers
  submit, editors review or ask for changes, and articles can be **scheduled**
  with `publish_at` (at most a year ahead, since a date further out is more
  likely a typo). Database `CHECK` constraints make an invalid state
  impossible to store.
- **Embeds**: tactics boards play inline using `@defer (on viewport)`, so
  nothing of Three.js loads until a reader scrolls to one. Videos and images
  are embedded from allow-listed sources.
- **Readable slugs** get a short random suffix from an alphabet without
  confusable characters (no 0/o or 1/l).

### 8.3 Member notes

- Any signed-in member can write **one note per player**. The unique
  constraint is what keeps a range of opinions from becoming spam.
- Reading notes is public, because the commentary is part of the product.

### 8.4 The Transfermarkt data importer (`cmd/import-transfermarkt`)

- Reads the open **transfermarkt-datasets** CSVs: clubs, players, transfers,
  valuations, games and appearances, about 65 MB gzipped. They are streamed
  and filtered to the top 10 UEFA leagues plus their cups and the European
  competitions.
- **One transaction**: it either fully succeeds or leaves the database as it
  was. A `-dry-run` flag runs everything, prints a report, then rolls back.
- **Bulk writes** use `unnest()` with Postgres arrays, in batches of 5,000
  rows. The full import takes about 20 seconds.
- **Idempotent** via an `external_refs` table that maps (source, kind,
  external id) to a local UUID. Running it again updates rows; it never
  duplicates them.
- **Matches existing hand-entered data** by normalised name. Club names have
  legal-form words such as "FC" and "SK" stripped; players are matched by name
  plus date of birth. Matched rows keep hand-entered fields such as notes and
  squad numbers, while factual fields are corrected.
- **Classifies transfers** with tested rules. A zero-fee move that reverses
  within two years is a loan and its return; "Retired" becomes a retirement; a
  move from a youth team becomes a promotion.
- **Aggregates appearances** into per-season stat lines for each player,
  competition and club. These are shown in a Performance table on the player
  page.
- **Security**: the database URL is read only from an environment variable,
  never from a command-line flag, so a password never ends up in shell
  history.

---

## 9. Infrastructure and deployment

### 9.1 Local development

- `make keys` generates the RSA key pair in Go, because openssl isn't
  installed on Windows by default.
- `make up` starts Postgres, NATS, two **one-shot migration jobs**, both
  services, and the Caddy gateway on `:8000`.
  - Startup order uses **health checks**, not just "the container started":
    the services wait for the migrations to finish, and the gateway waits for
    the services.
- **One Postgres instance, with one database and one login role per service.**
  No service can read another's tables, without needing a container per
  service.
- Prometheus, Jaeger and the frontend container are behind Compose
  **profiles**, so the default stack stays small.

### 9.2 Production

| Piece | Where | Notes |
|---|---|---|
| Frontend | **Vercel** | Static assets on the CDN; SSR runs as a **serverless function** (`api/ssr.mjs`) |
| API routing | **Vercel rewrites** | `/api/football/*`, `/api/identity/*` and the JWKS URL are proxied to Railway, so production plays the gateway's role |
| identity-svc, football-svc | **Railway** | Built from the Dockerfiles, with the repository root as the build context |
| Databases | **Supabase Postgres** | Two databases, one per service |
| Events | NATS | Optional; production currently runs without it |

**Getting Angular SSR onto Vercel** took the most work. Vercel's Angular preset
publishes a *static* site, but SSR output contains no `index.html`, so the
first deploy built fine and then returned 404 for every URL. I wrote a small
adapter function that loads the server bundle through a *runtime* dynamic
import (so esbuild doesn't inline it and break the lazy route chunks), and
rewrote every non-file route to that function.

### 9.3 CI/CD (GitHub Actions)

1. **A matrix with one job per Go module**: check `go mod tidy`, gofmt, build,
   `go vet`, golangci-lint, then `go test -race -short`.
2. **Migrations**: up, down and up again against a real Postgres, plus
   validating the Compose file.
3. **Integration tests**: testcontainers suites and black-box tests against the
   whole Compose stack. A failure-summary step reprints just the failing
   assertions at the end of the log.
4. **Frontend**: `npm ci`, a Prettier check, the production build (which is
   also the template type check and enforces bundle budgets), Vitest, then
   **SSR assertions**. CI starts the server and checks that:
   - the HTML contains the rendered shell and the OG and canonical tags;
   - `/account` is *not* rendered on the server;
   - an unknown URL returns 404 and a real page returns 200.
5. **The frontend Docker image** is built, to catch a Dockerfile that has
   drifted from the build.

### 9.4 Operations scripts

- `backup-db.ps1` and `restore-db.ps1` run `pg_dump` and `pg_restore` for both
  production databases. Credentials are pulled at run time through the Railway
  CLI, so they are never stored.
- `seed-demo.ps1` loads demo data, `check-endpoints.ps1` runs a smoke check
  against the endpoints, and `smoke-test.sh` / `smoke-test.ps1` do the same
  on either OS.

---

## 10. Testing strategy

| Level | What | Tools |
|---|---|---|
| Unit (Go) | Services with fake repositories; money, paging and errors; authorizer rules; importer classification; article body validation; JWT hardening; rate limiter; middleware | `testing`, testify |
| Integration (Go) | Repositories and full flows against real Postgres | testcontainers-go |
| Stack (Go) | Black-box HTTP tests through the gateway against the Compose stack | `tests/integration` |
| Unit (TS) | Single-flight refresh, permissions, money pipe, dates, `ssrResource`, board geometry, playback, the editor store, the article document | Vitest |
| SSR | The rendered HTML, status codes and render modes | curl in CI |

There are 206 Go test functions, many of them table-driven, and 112 frontend
specs.

**What I test hardest is behaviour that fails silently**: the refresh race,
SSR falling back to client rendering, soft 404s, and migrations that can't be
rolled back.

---

## 11. Mistakes I made and what I learned

- **Missed production migrations.** Supabase migrations are applied by hand,
  and I deployed code that queried columns which didn't exist yet, three times
  (`000009`, `000010`, `000011`). Every player page returned a 500, and every
  player showed as "Unknown player". Lesson: schema and code must ship
  together. The fix I'd make next is to have each service apply pending
  migrations at startup, or as a release step.
- **Grants inside the JWT** froze permissions for the token's lifetime. I moved
  to a per-request lookup with cache invalidation.
- **Shared HMAC secret** meant every verifier could also forge tokens. I moved
  to RS256 with JWKS.
- **A refresh token in localStorage** was readable by any injected script. I
  moved it to an HttpOnly cookie, and kept a one-time migration path for
  clients that still held a token in storage.
- **SSR "doing nothing"** turned out to be the Host header allow-list. I added
  a CI assertion on the rendered markup so it can't come back silently.
- **Expired rows piling up**: a `DeleteExpired` method existed and was stubbed
  in tests, but nothing called it. I added cleanup jobs.
- **Vitest showed "no tests" on Windows** rather than an error. I switched the
  pool from `forks` to `threads`, which also made the run 7× faster.

---

## 12. What I would do next

1. **Run migrations automatically on deploy**, for example
   `migrate up` at startup with an advisory lock.
2. **Batch lookups** (`GET /teams?ids=` or `?expand=`) to replace the
   load-everything `LookupStore`.
3. **A transactional outbox** for events, so delivery is at least once.
   That would make projections and notifications reliable.
4. **Scheduled data refreshes** from the dataset, and coaches in the importer.
5. **More match data**, which the schema deliberately leaves out today:
   league tables, fixtures and results.
6. **Redis-backed rate limiting** once a service runs more than one instance,
   since the in-process buckets are per instance.

---

## 13. Likely interview questions, short answers

**Why microservices for an app this size?**
Identity and football data change for different reasons and carry different
risks. Only identity holds the signing key and the passwords. Splitting them
taught me service boundaries, token verification across services and events.
I kept the cost low with a shared platform library and a service template, and
I'd still start most projects as a monolith.

**How does one service trust another's tokens?**
Asymmetric JWTs. identity-svc signs with RSA, and football-svc verifies with
the public key from JWKS. A verifier can't mint tokens, and the algorithm is
pinned during validation.

**How do you keep the squad consistent with transfers?**
A transfer is the only way to move a player. It writes the history row and
updates the derived `team_id` in one transaction, with the player row locked.

**How did you make SSR and auth work together?**
Public pages render anonymously on the server, which is exactly what should be
indexed. Pages that need a session render client-side only. The browser
restores the session from the HttpOnly cookie before the first route runs.

**What happens when the access token expires mid-page?**
Concurrent 401s share one refresh promise, then all retry. Without that, refresh
token reuse detection would log the user out everywhere.

**How do you prevent XSS in user-written articles?**
The body is a typed document tree rather than HTML. The server validates every
node and URL against an allow-list, the reader checks again while rendering,
and a hash-based CSP blocks any inline script that isn't on the list.

**How is the importer safe to re-run?**
An `external_refs` table maps source ids to local ids, so every row is
upserted. The whole import runs in a single transaction, and `-dry-run` rolls
back after printing the report.

**Why no ORM and no DI framework?**
I wanted explicit SQL, so I could reason about transactions, locks and
indexes, and explicit wiring, where a missing dependency is a compile error.
The standard library and sqlx covered everything I needed.

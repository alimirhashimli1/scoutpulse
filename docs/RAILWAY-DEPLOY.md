# ScoutPulse — Railway deployment brief

Context for an assistant helping deploy this repo to Railway. Everything below
is verified against the code, not assumed.

**Where we are:** two Postgres services created on Railway. Nothing else exists
yet. The application code still contains three things that must change before a
deploy can work — see "Blockers" below.

---

## 1. What the project is

A football scouting database. Two Go backend services, one Angular frontend
rendered on the server, behind a Caddy gateway.

- **Go 1.25**, one Go workspace (`go.work`) spanning 6 modules
- **Angular 21.2** with SSR (an Express/Node process, _not_ static files)
- **Postgres 16**
- **NATS** for events — optional, see below
- Local development runs on Docker Compose

## 2. Repo layout

```
/
  go.work                       Go workspace — this is why builds need the repo root
  docker-compose.yml            local dev only
  .dockerignore                 already present and correct
  deploy/gateway/Caddyfile      gateway routing rules
  apps/
    identity-svc/               Go — accounts, login, JWT issuing
      Dockerfile
      migrations/               000001..000004
    football-svc/               Go — players, clubs, competitions, transfers
      Dockerfile
      migrations/               000001..000008
    frontend/                   Angular SSR
      Dockerfile
  libs/
    auth/  db/  platform/       shared Go modules
```

**Critical build detail:** every Dockerfile expects the **repo root** as its
build context, because the Go modules import `libs/` which sits outside each app
folder. On Railway this means:

- Root Directory: leave as `/` (empty)
- Dockerfile Path: `apps/identity-svc/Dockerfile` (etc.)

Setting Root Directory to `apps/identity-svc` **will break the build.**

## 3. The services and their ports

| Service      | Image source                                    | Listens on            | Public?                   |
| ------------ | ----------------------------------------------- | --------------------- | ------------------------- |
| gateway      | Caddy — **no Dockerfile yet, must be written**  | `:8000` hardcoded     | **Yes — the only public** |
| identity-svc | `apps/identity-svc/Dockerfile`                  | `:8080` hardcoded     | No                        |
| football-svc | `apps/football-svc/Dockerfile`                  | `:8081` hardcoded     | No                        |
| frontend     | `apps/frontend/Dockerfile`                      | `PORT` env, def. 4000 | No                        |
| identity-db  | Railway PostgreSQL                              | —                     | No                        |
| football-db  | Railway PostgreSQL                              | —                     | No                        |

Only the frontend reads `PORT`. The two Go services and Caddy have their ports
written into the source. Railway can be told a fixed target port per service in
Settings → Networking, so this is workable without code changes for the private
services; the **public** gateway is the one that genuinely needs `PORT`.

## 4. Target topology

```
Internet
   |
   v
gateway  (Caddy, public domain)
   |-- /.well-known/jwks.json  -> identity-svc:8080
   |-- /api/identity/*         -> identity-svc:8080   (prefix stripped)
   |-- /api/football/*         -> football-svc:8081   (prefix stripped)
   `-- /*                      -> frontend:4000       (NOT WIRED YET)

identity-svc -> identity-db
football-svc -> football-db
```

Railway private DNS is `<service-name>.railway.internal`. So the Caddyfile's
`identity-svc:8080` becomes `identity-svc.railway.internal:8080`.

**Why the gateway must also serve the frontend:** it makes the browser
same-origin with the API. That removes the hardcoded API URL problem (Blocker
A), the CORS configuration, and cross-site cookie issues with OAuth refresh
tokens — all at once. Currently the Caddyfile returns 404 at the catch-all.

## 5. Environment variables

### identity-svc

| Variable                                                          | Value on Railway                                                                          |
| ----------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| `DB_HOST`                                                         | reference `identity-db` PGHOST                                                            |
| `DB_PORT`                                                         | `5432`                                                                                    |
| `DB_USER`                                                         | reference `identity-db` PGUSER                                                            |
| `DB_PASSWORD`                                                     | reference `identity-db` PGPASSWORD                                                        |
| `DB_NAME`                                                         | `railway`                                                                                 |
| `DB_SSLMODE`                                                      | `disable` (private network) — code already defaults to `disable`                          |
| `JWT_PRIVATE_KEY`                                                 | **from the developer's local `.env`** — RSA private key, PEM                              |
| `PUBLIC_BASE_URL`                                                 | `https://<gateway-domain>/api/identity`                                                   |
| `FRONTEND_URL`                                                    | `https://<gateway-domain>`                                                                |
| `CORS_ALLOWED_ORIGINS`                                            | `https://<gateway-domain>`                                                                |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`                       | optional — absent means Google sign-in is simply not offered                              |
| `FACEBOOK_CLIENT_ID` / `FACEBOOK_CLIENT_SECRET`                   | optional, same                                                                            |
| `SMTP_HOST` `SMTP_PORT` `SMTP_USERNAME` `SMTP_PASSWORD` `SMTP_FROM` | optional — with no `SMTP_HOST` the verification link is written to the log, not emailed |
| `CAPTCHA_PROVIDER` `CAPTCHA_SITE_KEY` `CAPTCHA_SECRET`            | optional — absent secret skips the check                                                  |
| `NATS_URL`                                                        | omit (see below)                                                                          |

### football-svc

| Variable                                 | Value                                                                                       |
| ---------------------------------------- | ------------------------------------------------------------------------------------------- |
| `DB_HOST` `DB_PORT` `DB_USER` `DB_PASSWORD` | reference `football-db`                                                                  |
| `DB_NAME`                                | `railway`                                                                                   |
| `DB_SSLMODE`                             | `disable`                                                                                   |
| `JWT_PUBLIC_KEY`                         | **from local `.env`** — the matching RSA _public_ key. This service cannot mint tokens, only verify them. |
| `CORS_ALLOWED_ORIGINS`                   | `https://<gateway-domain>`                                                                  |
| `NATS_URL`                               | omit                                                                                        |

### frontend

| Variable               | Value                                                                                                                                             |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GATEWAY_INTERNAL_URL` | `http://gateway.railway.internal:8000` — used during server rendering                                                                             |
| `SITE_URL`             | `https://<gateway-domain>` — canonical links, og:url                                                                                              |
| `NG_ALLOWED_HOSTS`     | must include the gateway domain. **If the Host header does not match, Angular silently falls back to client-side rendering** — the app appears to work but renders nothing server-side. |

### About NATS

Publishing events is optional and degrades cleanly
(`libs/platform/events/nats.go` returns a no-op publisher when `NATS_URL` is
unset). Skip it initially.

**The real cost of skipping it:** role changes and account deletions in
identity-svc are broadcast over NATS, and football-svc subscribes so that editor
permissions do not outlive the account. Without NATS, a demoted or deleted user
keeps their editor grants in football-svc. Acceptable for a beta with few users;
not acceptable long-term.

---

## 6. Blockers — code changes still required

These are **not done yet**. A deploy without them builds successfully and then
fails at runtime, which is the worst failure mode.

### A. The browser bundle hardcodes localhost

`apps/frontend/src/app/core/tokens/api-config.ts` line 32:

```ts
export const BROWSER_GATEWAY = 'http://localhost:8000';
```

This is compiled into the JavaScript sent to visitors. Every API call from the
browser would target the visitor's own machine. Server-side rendering would work
(it uses `GATEWAY_INTERNAL_URL`), so the site looks fine until you click
something.

**Fix:** route the frontend through the gateway (section 4) and make the browser
API base a relative path, so no absolute URL is compiled in at all.

### B. The Caddyfile is written for Docker Compose

`deploy/gateway/Caddyfile` needs three changes:

1. `:8000` → `:{$PORT}` — Railway assigns the port for the public service
2. `identity-svc:8080` → `identity-svc.railway.internal:8080` (same for football)
3. The catch-all `handle { ... 404 }` block → `reverse_proxy frontend.railway.internal:4000`

Also, Compose mounts the Caddyfile as a **volume**. Railway cannot mount files,
so the gateway needs its own small Dockerfile that copies the Caddyfile into a
`caddy:2.8-alpine` image. **This Dockerfile does not exist yet and must be
written.**

### C. Migrations have no way to run

Compose runs migrations with a separate `migrate/migrate` container. Railway
never runs that. Without migrations the databases are empty and both services
fail on their first query.

Three options, in order of preference:

1. **Railway pre-deploy command** on each Go service. Requires the `migrate`
   binary inside the image — the Dockerfiles currently produce a minimal
   `alpine:3.19` image containing only the compiled binary, so this needs a
   Dockerfile change.
2. **Run migrations from the developer's machine** against each database's
   public URL, once, using the `migrate` CLI in Docker. Simplest to get going;
   must be repeated by hand on every future migration.
3. **Run migrations at service startup.** Convenient but risky with more than
   one instance — two containers starting together can race on the same
   migration.

---

## 7. Suggested order

1. ~~Create two Postgres services, rename to `identity-db` and `football-db`~~ **done**
2. Fix blockers A and B, write the gateway Dockerfile, commit and push
3. Create the `identity-svc` service (Dockerfile path, variables), deploy
4. Create `football-svc` the same way
5. Run migrations (blocker C) — both databases
6. Create `frontend`
7. Create `gateway`, generate its public domain, then go back and fill
   `PUBLIC_BASE_URL`, `FRONTEND_URL`, `CORS_ALLOWED_ORIGINS`, `SITE_URL` and
   `NG_ALLOWED_HOSTS` with that domain
8. If using Google/Facebook sign-in, add the new callback URL to each provider's
   console:
   `https://<gateway-domain>/api/identity/api/v1/auth/oauth/<provider>/callback`
9. Smoke test: register, find the verification link in the identity-svc logs,
   sign in, open a player page

## 8. Notes

- Cost: 6 Railway services. The SSR frontend is the heaviest. The Hobby plan's
  $5 monthly credit will be tight.
- The repo is **public**. A real password was committed to it earlier in its
  history and must not be reused for anything deployed.
- `.env` is correctly gitignored. The JWT keys exist only on the developer's
  machine — if lost, all issued tokens become unverifiable and every user is
  signed out.

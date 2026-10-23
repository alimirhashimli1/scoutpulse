# Improvements backlog

Findings from a code scan of the Go services, shared libraries, Angular frontend and deploy config (2026-10). Items already fixed in [ISSUES.md](ISSUES.md) are not repeated here.

Severity: **High** / **Medium** / **Low**.

---

## Security

### S1. Login rate limit can be bypassed — High
- **Status:** fixed. Rate limits read the IP from `CLIENT_IP_HEADER` (default: `X-Real-IP` on Railway, the TCP peer elsewhere, `X-Forwarded-For` only in compose behind Caddy). Failed sign-ins are also capped per account (10, then 1 per 3 min), whatever IP they come from. Still open: routing production through the gateway instead of the public Railway domains.
- **Where:** [apps/frontend/vercel.json](apps/frontend/vercel.json), [libs/platform/server/ratelimit.go](libs/platform/server/ratelimit.go) (lines ~128–150)
- **Problem:** Vercel rewrites API traffic straight to the services' public Railway domains, skipping Caddy. The rate limiter trusts the first `X-Forwarded-For` value on the assumption that only the gateway can reach the services. In production anyone can call `/api/v1/auth/login` directly with a fake `X-Forwarded-For` on each request and get unlimited password guesses (captcha only helps if configured).
- **Fix:** route all traffic through the gateway and remove the services' public domains, **or** only trust `X-Forwarded-For` from known proxy addresses (and set `trusted_proxies` in Caddy).

### S2. `/metrics` probably public in production — Medium
- **Status:** fixed. The services guard `/metrics` themselves: 404 on Railway unless `METRICS_TOKEN` is set and sent as a bearer token.
- **Where:** [apps/football-svc/main.go](apps/football-svc/main.go) (~line 144), identity-svc equivalent
- **Problem:** only the Caddyfile blocks `/metrics`. The direct Railway domains expose request volumes and latency.
- **Fix:** covered by S1; additionally protect the endpoint in the services themselves (token, or a separate internal port).

### S3. Refresh token in `localStorage`, no security headers — Medium
- **Status:** fixed. The SSR server sends a hash-based CSP (`script-src` allows only this origin, the captcha providers and the page's known inline scripts), plus `frame-ancestors 'none'`, X-Frame-Options, nosniff, Referrer-Policy and Permissions-Policy. The refresh token now lives in an HttpOnly, Secure, SameSite=Strict cookie scoped to the auth endpoints; only a non-secret "signed in" flag stays in localStorage. Tokens stored by the old version are moved onto the cookie on the next visit.
- **Where:** [apps/frontend/src/app/core/auth/token-storage.ts](apps/frontend/src/app/core/auth/token-storage.ts), `server.ts`, `vercel.json`
- **Problem:** a 30-day refresh token is readable by any injected script. There is no CSP, `frame-ancestors` or `X-Frame-Options`.
- **Fix:** add CSP and frame headers now. Longer term, move the refresh token to an HttpOnly cookie on the identity origin.

### S4. "Members only" tactics boards are effectively public — Medium
- **Status:** fixed. Members see only their own boards; admins see everyone's. Sharing by link is unchanged.
- **Where:** [apps/football-svc/internal/service/tactic_service.go](apps/football-svc/internal/service/tactic_service.go) (~lines 96–101)
- **Problem:** `GET /api/v1/tactics` with no `author_id` lists every member's boards, and anyone can sign up, so a free account can browse and open every board.
- **Fix:** list only the caller's own boards unless the caller is an admin.

### S5. No quota or rate limit on member writes — Low–Medium
- **Status:** fixed. Max 500 boards per account; every authenticated football-svc write is limited per account (burst 60, then 2/s).
- **Problem:** a verified member can create unlimited tactics boards of up to 512 KB each. Rate limits exist only on identity login and registration.
- **Fix:** per-user board cap and a per-user write rate limit in football-svc.

### S6. Gemini key (open from ISSUES.md) — Low
- **Status:** open — manual step in Google AI Studio.
- **Fix:** rotate the Gemini key in `.env` if it has ever left this machine.

---

## Bugs that hurt users

### B1. Multi-tab refresh can log the user out everywhere — High
- **Status:** mostly fixed with S3. Tabs share the cookie; a token reused within 30 s of rotation is refused without revoking every session, and the refused tab retries once with the rotated cookie.
- **Where:** [apps/frontend/src/app/core/auth/auth-interceptor.ts](apps/frontend/src/app/core/auth/auth-interceptor.ts), [auth-facade.ts](apps/frontend/src/app/core/auth/auth-facade.ts) (~line 86), [apps/identity-svc/internal/handler/auth.go](apps/identity-svc/internal/handler/auth.go) (~line 328)
- **Problem:** refresh dedupe only works within one tab, but all tabs share one rotating refresh token. When several tabs refresh at once:
  - the losing tab gets a 401 and runs `session.clear()`, deleting the token the winner just saved; or
  - it arrives after the winner, looks like token reuse, and the server revokes every session for the user.
- **Fix:** coordinate tabs with `navigator.locks` or `BroadcastChannel`; give the backend a few seconds' grace for a just-rotated token.

### B2. Any startup failure signs the user out — Medium
- **Status:** fixed with S3. Only a real 401 clears the session; network errors and cold starts leave it for the next load.
- **Problem:** `restore()` catches every error and calls `session.clear()`, including network errors and Railway cold-start 5xx.
- **Fix:** clear the session only on a real 401.

### B3. App start waits on the session check — Low–Medium
- **Problem:** startup awaits refresh then profile, two sequential network calls, before the page becomes interactive.
- **Fix:** don't block startup on them, or at least fetch the profile in the background.

---

## Performance

### P1. Pages load every club to show a few names — High (as data grows)
- **Where:** [apps/frontend/src/app/core/api/lookup-store.ts](apps/frontend/src/app/core/api/lookup-store.ts) (~line 120)
- **Problem:** `loadTeams()` pages through every team 100 at a time, sequentially, silently stopping at 5,000, and embeds them all in the SSR HTML. Used by player, transfer feed, coach and competition pages. 2,000 clubs means 20 Vercel→Railway requests before first byte; names past 5,000 show "—".
- **Fix:** return `team_name` / `league_name` alongside ids from the API (model on the existing `PlayerNames` batched lookup).

### P2. Search isn't typo-tolerant; trigram indexes unused
- **Where:** [apps/football-svc/internal/repository/search_repository.go](apps/football-svc/internal/repository/search_repository.go)
- **Problem:** WHERE filters only by full-text match; trigram similarity is used only for ranking. "mesi" / "barcelna" return nothing.
- **Fix:** add `OR p.name % $2` (and equivalents) to each WHERE clause.

### P3. No caching
- **Problem:** no `Cache-Control` / ETag on public API reads or SSR HTML; every view runs a Vercel function that calls Railway.
- **Fix:** `s-maxage=60, stale-while-revalidate=300` on public pages and reads.

### P4. No response compression
- **Problem:** neither Caddy nor the Go services compress; tactics boards are up to 512 KB of JSON.
- **Fix:** `encode zstd gzip` in Caddy, or compression middleware in `libs/platform/server`.

### P5. 3D tactics board renders continuously
- **Where:** [apps/frontend/src/app/features/tactics/board/engine.ts](apps/frontend/src/app/features/tactics/board/engine.ts) (~line 892)
- **Problem:** redraws at 60 fps with soft shadows even when idle; WebGL context isn't released on dispose.
- **Fix:** render on demand (playback, camera moves, drags, selection ring); call `renderer.forceContextLoss()` in `dispose()`.

### P6. Minor
- Offset paging gets slow deep in big lists; consider keyset paging for the transfer feed.
- Tactics updates have no conflict check; two tabs silently overwrite each other. Add an `updated_at` check.

---

## User experience

- **U1. Search as you type:** search runs only on Enter. Add a debounced suggestion dropdown (≈5 results); the endpoint already supports it.
- **U2. Mobile nav:** links just wrap below 40rem. Add a collapsible menu.
- **U3. Return after OAuth sign-in:** [oauth-callback.ts](apps/frontend/src/app/features/auth/oauth-callback.ts) (~line 74) always lands on `/` and drops `returnTo`; password sign-in keeps it.
- **U4. Tactics editor autosave:** draft is saved only when redirected to sign in. Autosave locally while editing.
- **U5. Wrong "can't edit" message:** [tactics-editor.ts](apps/frontend/src/app/features/tactics/editor/tactics-editor.ts) (~line 184) tells the owner they can't edit while the profile is still loading.
- **U6. Missing club/league names** past the 5,000-club cap — fixed by P1.

---

## Suggested order

1. S1 + S2 — gateway routing and IP trust
2. B1 + B2 — multi-tab refresh and sign-out only on 401
3. P1 — names with ids from the API
4. P2 + U1 — typo-tolerant search and live suggestions
5. P3 + P4 — caching and compression
6. S3, S4, S5 — security headers, private tactics list, write limits

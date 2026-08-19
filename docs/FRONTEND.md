# ScoutPulse frontend — architecture and build log

Angular 21 · standalone components · signals · vanilla CSS

This is both the design and the progress tracker. Every step is listed once,
with its status. Update the status as you go rather than keeping a second list
somewhere.

**Status legend:** `[ ]` not started · `[~]` in progress · `[x]` done

---

## 1. What we are building against

The backend is finished for this phase. Numbers, not estimates:

| | |
|---|---|
| Endpoints | 67 (51 football, 16 identity) |
| Entry point | `http://localhost:8000` — one origin, gateway routed |
| Football prefix | `/api/football/api/v1/...` |
| Identity prefix | `/api/identity/api/v1/...` |
| Auth | RS256 bearer, 15-minute access token, rotating refresh |
| External sign-in | Google, Facebook (redirect flow) |

`CORS_ALLOWED_ORIGINS` already permits `http://localhost:4200`, so `ng serve`
talks to the gateway with no configuration.

### What exists, and therefore what we can build

| Page | Data available? |
|---|---|
| Search (quick + results) | Yes — `/search?q=&kind=` |
| Player profile: bio, club, career, value chart | Yes |
| Club page: squad, staff, competition history | Yes |
| Transfer feed with filters | Yes |
| Competition page: clubs in a league | Yes |
| Coach profile with career | Yes |
| Admin: users, roles, editor grants | Yes |
| **League table / standings** | **No — no match data** |
| **Fixtures, results, match reports** | **No** |

The absence of match data is deliberate and decided. Do not design navigation
that implies a standings page exists.

---

## 2. Five backend behaviours that dictate the design

These are not preferences. Getting any of them wrong produces bugs that are
hard to trace back to their cause.

### 2.1 Refresh rotates, and reuse revokes everything

A refresh token is **single use**. Presenting one that has already been
exchanged is treated as a leaked credential, and **every session for that user
is destroyed**.

Three requests 401 at the same moment — which is normal when a token expires
mid-page — and a naive interceptor fires three refreshes. The first succeeds;
the other two present a spent token; the user is silently logged out
everywhere.

**The interceptor must single-flight the refresh:** the first 401 starts it,
every other request waits on that same in-flight call. This is the single most
important thing in the frontend.

### 2.2 Lists are envelopes, never bare arrays

```json
{ "items": [], "limit": 25, "offset": 0, "has_more": false }
```

Every list endpoint. `limit` is clamped to 100 server-side however much you
ask for. Build the generic `Page<T>` type first and have every list component
consume it — retrofitting this later touches every list in the app.

### 2.3 Money is an integer count of minor units

`2500` means €25.00. Never a decimal. The API **rejects** decimals rather than
rounding them, because 1.5 cents is ambiguous.

Large fees exceed JavaScript's safe integer range, so parse defensively and
format through one pipe. Never do arithmetic on these in the UI.

### 2.4 A player's club changes only through a transfer

`PUT /players/{id}` accepts a `team_id` and **silently ignores it**. The club
is derived from transfer history.

So the player edit form must not show a club field. Moving a player is a
different action, on a different form, that posts a transfer. Designing this
correctly is a UX decision the API is forcing, and it is the right one.

### 2.5 Errors have a fixed shape

```json
{ "error": "human message", "code": "invalid", "request_id": "..." }
```

Codes: `invalid` · `unauthorized` · `forbidden` · `not_found` · `conflict` ·
`rate_limited` · `internal`. Handle them centrally, once, and show
`request_id` on internal errors so a user can quote it.

---

## 3. Architecture

### 3.1 Folder shape

```
apps/frontend/src/app/
  core/                         singletons, provided once at the root
    api/                        HTTP plumbing, one file per resource
    auth/                       session state, interceptor, guards
    models/                     interfaces mirroring the wire types
    tokens/                     InjectionTokens for the abstractions
  shared/                       reusable, no feature knowledge
    ui/                         button, card, pill, empty-state, spinner
    pipes/                      money, date, position
    pagination/                 one paginator, driven by Page<T>
  features/                     one folder per area, lazily loaded
    search/  players/  clubs/  transfers/  competitions/
    coaches/  account/  admin/
  layout/                       shell, header, footer, nav
```

**The rule that keeps this scalable:** dependencies point *inwards only*.
`features` may import from `shared` and `core`. `shared` may import from
`core`. `core` imports from neither. A feature **never** imports from another
feature — if two need the same thing, it moves to `shared`.

### 3.2 The layers

```
Component     renders, and raises intent. No HTTP, no business rules.
   ↓
Facade        one per feature. Holds signals, orchestrates. What the
              component talks to.
   ↓
Repository    one per resource. Speaks HTTP, returns typed models.
   ↓
HttpClient    interceptors: auth, error, correlation id
```

A component that injects `HttpClient` is a bug. It makes the component
untestable without a network stub and puts request knowledge in a template.

---

## 4. SOLID, applied concretely

Not as slogans — as the specific decisions each principle produces here.

### Single responsibility

Four kinds of file, each with one job:

| Kind | Does | Never does |
|---|---|---|
| Component | Renders state, emits intent | HTTP, business rules |
| Facade | Holds signals, orchestrates | Renders, builds URLs |
| Repository | HTTP for one resource | State, decisions |
| Mapper | Wire shape ↔ view model | Anything else |

The test: *what change forces me to edit this file?* If a component changes
for both a design tweak and an API rename, it is doing two jobs.

### Open/closed

Adding an entity — say competitions gain a "trophies" tab — means adding a
feature folder and a repository. It must not require editing `core`, the
interceptor, or another feature.

The mechanism: generic `Page<T>` and a `ReadRepository<T>` base, so a new
resource inherits paging and error handling rather than reimplementing it.

### Liskov substitution

Every repository is consumed through its interface. `PlayerRepository` is
a *contract*; `HttpPlayerRepository` and `InMemoryPlayerRepository` (for tests
and Storybook) are interchangeable. A facade must not be able to tell which it
has.

This is what makes component tests fast: swap the provider, no HTTP mocking.

### Interface segregation

**No `ApiService` with sixty methods.** One narrow interface per resource, and
split further by capability where the split is real:

```ts
interface PlayerReader { list(q): Promise<Page<Player>>; byId(id): Promise<Player>; }
interface PlayerWriter { create(p): Promise<Player>; update(p): Promise<Player>; }
```

The public player page injects `PlayerReader` only. It then *cannot* call a
write method — the compiler enforces the read-only page, rather than a code
review noticing.

### Dependency inversion

Components and facades depend on `InjectionToken`s, never on concrete classes.

```ts
export const PLAYER_READER = new InjectionToken<PlayerReader>('PlayerReader');
// bootstrap: { provide: PLAYER_READER, useClass: HttpPlayerRepository }
```

Swapping transport — HTTP to a mock, or a future GraphQL gateway — changes one
provider line, not every consumer.

---

## 5. Auth design

The part most likely to go wrong, so it is specified rather than left to
improvisation.

### Session state

A single `SessionStore` holding signals: `user`, `accessToken`, `status`.
Everything else derives from it — `isAdmin`, `isEditor`, `isAuthenticated` are
`computed()`, never separate fields that can disagree.

### Token storage

| Token | Where | Why |
|---|---|---|
| Access | Memory only | Lives 15 minutes. In memory it dies with the tab and cannot be read by an injected script. |
| Refresh | `localStorage` | Must survive a reload, or every refresh logs the user out. |

This is a deliberate trade-off, not an oversight: a refresh token in
`localStorage` is reachable by XSS, but the alternative — an HttpOnly cookie —
means a second auth mechanism alongside the bearer tokens every other endpoint
uses. Revisit if the app ever renders untrusted content.

### The interceptor — single-flight refresh

```
request → attach access token if present
        → 401?
            → is a refresh already in flight?
                 yes → wait for it, then retry with the new token
                 no  → start ONE refresh, queue everything else on it
            → refresh failed → clear session, redirect to /login
        → any other status → pass through
```

Never refresh on the refresh endpoint itself, or a failure loops forever.

### OAuth

```
user clicks "Sign in with Google"
  → browser navigates to  /api/identity/api/v1/auth/google   (full page, not XHR)
  → provider consent
  → backend callback resolves the account
  → browser lands on  /auth/callback?code=XXX
  → app POSTs { code } to /api/identity/api/v1/auth/exchange
  → normal token pair, session started
```

The `code` is **single use and expires in 60 seconds**, so exchange it
immediately on route activation. A failed sign-in arrives as
`/auth/callback?error=REASON` — handle `email_taken` specifically: it means an
account already holds that email and the user should sign in with their
password, then link the provider from settings.

Which providers to show comes from `GET /auth/providers` — do not hardcode
buttons that may not be configured.

### Guards

| Guard | Allows |
|---|---|
| `authGuard` | any signed-in user |
| `adminGuard` | role `admin` |
| `editorGuard` | role `admin` or `editor` |

Guards are for **navigation**, not security. The API enforces every rule
independently; a guard only avoids showing a page that would 403.

---

## 6. Routes

```
public, server-rendered
  /                             transfer feed (landing)
  /search                       results, ?q= and ?kind=
  /players/:id                  profile · career · value chart
  /clubs                        all clubs
  /clubs/:id                    squad · staff · competition history
  /competitions                 all
  /competitions/:id             clubs in it
  /coaches/:id                  profile · spells
  /transfers                    feed with filters
  /seasons                      list (writes are admin-only)

writes, client-rendered
  /players/new                                                [editorGuard]
  /players/:id/edit             no club, no market value      [editorGuard]
  /players/:id/transfer         the only way to move a player [editorGuard]
  /players/:id/values/new                                     [adminGuard]
  /transfers/:id/edit           type, fee, season only        [editorGuard]
  /clubs/new  /clubs/:id/edit                                 [adminGuard]
  /clubs/:id/seasons/new        enter a competition           [editorGuard]
  /competitions/new  /competitions/:id/edit                   [adminGuard]
  /seasons/new  /seasons/:id/edit                             [adminGuard]
  /coaches/new  /coaches/:id/edit                             [editorGuard]
  /coaches/:id/spells/new                                     [editorGuard]

private, client-rendered
  /login  /register  /auth/callback
  /account                      profile · password · linked providers
  /admin/users                  list, search, roles, deletion [adminGuard]
  /admin/clubs/:id/editors      grants                        [adminGuard]
```

Every feature route lazily loaded with `loadComponent`, so the initial bundle
carries the shell and nothing else.

**Order matters where a literal and a parameter share a prefix.** `clubs/new`
is declared before `clubs/:id`, or the form loads as a club whose id is the
word "new" — a 404 on a page that ought to work.

---

## 7. Build steps

### Phase 0 — foundations · **done**

- [x] 0.1 `ng new` into `apps/frontend` — Angular 21.2, standalone, **SSR**, vanilla CSS, Express 5
- [x] 0.2 Folder skeleton from §3.1
- [x] 0.3 `API_CONFIG` token, provided per platform (§11.3) — browser gets the public gateway, the Node renderer gets `GATEWAY_INTERNAL_URL`
- [x] 0.4 Providers: `provideHttpClient(withFetch())`, `provideClientHydration(withEventReplay())`, zoneless change detection, `withComponentInputBinding()`
- [x] 0.5 CSS foundation — tokens for colour, type and space; light and dark; almanac direction
- [x] 0.6 `app.routes.server.ts` — see the note below
- [x] 0.7 Verified: build succeeds, and the SSR server returns **rendered HTML containing page content**, having reached the API server-side

**Two things worth recording, both discovered by running it:**

**Angular validates server routes against the client router.** A `RenderMode`
entry for a path with no matching route is a build error. So the render-mode
split cannot be written up front — each entry is added in the same change as
its component. The intended split is documented in `app.routes.server.ts` so
it is not lost.

**Angular 21 checks the `Host` header, and silently degrades if it fails.**
The header is attacker-controlled and the renderer builds absolute URLs from
it, so an unchecked host allows server-side request forgery. With no
`allowedHosts` configured, Angular logs a warning and **falls back to
client-side rendering** — which presents as "SSR does nothing" with a fully
working build. `server.ts` now reads `NG_ALLOWED_HOSTS`, defaulting to
localhost and the compose service name. **Set it to the real domain in
production.**

`resource()` is used for data loading rather than a subscription, because it
participates in hydration — a server-rendered page does not re-request
everything the moment it reaches the browser.

### Phase 1 — core plumbing · **done**

- [x] 1.1 `models/football.ts`, `models/identity.ts` — wire types, field names identical to the JSON
- [x] 1.2 `Page<T>` with paging helpers, and `ApiError` with typed codes
- [x] 1.3 `HttpRepository` base — URL building, parameter serialisation, paging
- [x] 1.4 Reader/writer interfaces and an `InjectionToken` for each (§4); `provideFootballApi()` is the only file naming a concrete class
- [x] 1.5 `errorInterceptor` — every failure becomes an `ApiError` before a caller sees it
- [x] 1.6 `correlationIdInterceptor` — `X-Request-ID` known before the response lands
- [x] 1.7 `MoneyPipe` with 16 tests, including the compact form and undisclosed-vs-free
- [x] 1.8 `Paginator` driven by `Page<T>`

**Notes from building it:**

Reader and writer are separate *interfaces* but one class per resource, bound
with `useExisting`. The split buys the compile-time guarantee — a page holding
`PLAYER_READER` cannot call `create` — without instantiating two objects.

`Home` now injects `LEAGUE_READER` rather than `HttpClient`, so the whole chain
is exercised: token → contract → HTTP repository → interceptors → gateway. It
still stands in for the transfer feed until phase 3.

There is no total-count paging. The API reports `has_more`, not a total,
because counting every matching row on each request is expensive and nothing in
the product needs the number — so the paginator offers next and previous, and
shows a range rather than "page 4 of 27".

**Test runner:** the default Vitest `forks` pool timed out starting workers on
Windows and reported *"no tests"* rather than a failure. `vitest.config.ts`
switches it to `threads`, which also took the run from 55s to 8s. The generated
`app.spec.ts` asserted the starter template's greeting and was rewritten to
test what the shell actually does.

### Phase 2 — auth · **done**

- [x] 2.1 `SessionStore` — signals, with `isAdmin`/`isEditor` computed from the role rather than stored alongside it
- [x] 2.2 `AuthRepository` — register, login, refresh, logout, me, password change, providers, exchange, identities
- [x] 2.3 **Single-flight refresh interceptor** — 10 tests, including the concurrent-401 case
- [x] 2.4 Login and register pages
- [x] 2.5 `authGuard`, `adminGuard`, `editorGuard`
- [x] 2.6 `GET /auth/providers` → only configured buttons render
- [x] 2.7 `/auth/callback` — exchanges the one-time code, explains `email_taken`
- [x] 2.8 Session restore via `provideAppInitializer`, browser only

**The concurrency test, and what it actually proved.** Three simultaneous 401s
produce exactly **one** refresh, and all three retry with the renewed token.
Getting this wrong would have the backend see a re-presented refresh token,
conclude the credential leaked, and revoke every session the user holds —
random logouts, on page load, for no reason the user could see.

Two things the test caught in my own code:

- The in-flight promise has to live at **module scope**. Every request runs a
  fresh invocation of the interceptor function, so anything held inside it is
  gone before the next 401 arrives, and each would start its own refresh.
- The slot must be cleared in `finally`. Left set, a later 401 would reuse a
  stale result forever.

`SKIP_AUTH` marks the auth calls themselves, so a failing refresh cannot
trigger a refresh — that recursion presents as the tab hanging.

**Token storage** is split deliberately: the access token lives in memory only
(15 minutes, dies with the tab, unreadable from storage), the refresh token in
`localStorage` because it must survive a reload. `TokenStorage` is a no-op on
the server — reading storage during server rendering throws, and the error
points nowhere near the cause.

**Verified:** `/` is server-rendered with content; `/login` ships as a client
shell. The render-mode split is doing what §11.4 describes.

### Phase 3 — read-only app · **mostly done**

The whole browse experience, no writes.

- [x] 3.1 Layout shell — masthead, search, nav, footer, session-aware account area
- [~] 3.2 Search — full results page with kind filters. **No type-ahead dropdown yet**; the header search navigates to `/search`
- [x] 3.3 Transfer feed with type filters and paging
- [x] 3.4 Player profile — bio, club, career, value chart
- [x] 3.5 Value chart — inline SVG, no chart library
- [x] 3.6 Club page — squad and managerial history
- [x] 3.7 Competition list and detail
- [x] 3.8 Coach profile with spells
- [x] 3.9 Empty, loading and error states as shared components
- [~] 3.10 Responsive — breakpoints on the shell, search and tables; not swept end to end

**The join the API does not do.** A transfer carries `from_team_id`, not a
club, so rendering "Selling FC → Buying FC" needs a lookup. One request per row
would be an N+1 — twenty-five rows, fifty requests — so `LookupStore` loads the
club and competition lists once and caches them.

That works because both are small, and **it is the piece that will not scale**.
Past a few thousand clubs it should become an `?expand=` parameter or a batch
`GET /teams?ids=` endpoint. The trade is recorded in the file itself so the
limit is not rediscovered by surprise.

**Deliberate absences.** No standings, no fixtures, and no navigation implying
either. The competition page says "Clubs", not "Table", because there is no
match data to compute a standing from and a placeholder would be a lie.

**Honest gaps carried into phase 6:** the header search has no type-ahead
dropdown, the transfer feed filters by type but not yet by club or season, and
the responsive pass is partial. None blocks writes, so phase 4 can start.

**Verified:** `/`, `/clubs`, `/competitions` and `/search` all render
server-side with content; `/login` ships as a client shell. 28 tests pass.

### Phase 4 — writes · **done**

- [x] 4.1 Player create/edit — **no club field on edit** (§2.4), and no market value either
- [x] 4.2 Record a transfer — the only way to move a player
- [x] 4.3 Club create/edit — **the whole form is admin-only**, not just `league_id`
- [x] 4.4 Coach and spell forms
- [x] 4.5 Market value entry (admin only)
- [x] 4.6 Season management, plus entering a club in a competition
- [x] 4.7 Validation mirroring the server's rules, with server errors as the fallback
- [x] 4.8 Competition create/edit — not in the original list; see below

**Permissions are read, not guessed.** `GET /users/{id}/teams` reports which
clubs the caller may edit — the endpoint that replaced the `managed_team_ids`
token claim so revocation is immediate. `Permissions` loads it once per session
and mirrors the server's four rules:

| Rule | Who |
|---|---|
| `RequireTargetTeam` | admin, or an editor holding that club |
| A record belonging to **no club** | **admin only** — no grant can cover "no club" |
| `RequireEitherTeam` | either end of a move authorises it |
| Leagues, seasons, valuations, deletions | admin |

The second row is the one that shapes the UI: an editor cannot create a free
agent, so the club field on the player form is *restricted to clubs they hold*
rather than merely offered. The third means the transfer form's submit button
unlocks the moment a destination they manage is chosen, even when they hold
nothing at the selling end. Nine tests in `permissions.spec.ts` pin all four.

**Three corrections to what this file said before building it.**

*The create/edit split is wider than §2.4 recorded.* `PUT /players/{id}`
discards `team_id` **and** `market_value_minor` — both are derived, one from
transfer history and one from the valuation series. `POST /players` accepts
both and records them as history: an opening transfer and a first valuation.
So the club and value fields are offered on create and absent on edit. A field
that is accepted and thrown away is worse than no field.

*Club editing is entirely admin-only.* `CreateTeam` and `UpdateTeam` both call
`RequireAdmin`, so a grant does not let someone rename their own club. The
grant covers the club's *contents* — squad, transfers, competition entries.
This file previously said "`league_id` admin-only", which understated it.

*Competitions had no form at all.* The phase list went from clubs to coaches,
but the club form asks which competition a club is in — on a fresh database
that dropdown could only ever be empty. Added for that reason.

**The transfer form does not ask where the player is coming from.** The API
requires `from_team_id` to equal their current club exactly, and to be omitted
for a free agent. A selling-club dropdown would have one correct answer and
every other choice would be a round trip that fails, so the origin is stated as
a fact and sent from the loaded record.

**Dates and money each needed a converter, and both guard a real bug.** The
API's date fields are Go `time.Time`, so they want `1998-03-05T00:00:00Z` and
**reject** the `1998-03-05` a date input produces. Both directions work on the
string rather than through `new Date()`, because a Date parses a bare date as
UTC midnight and local getters read it back a day earlier west of Greenwich.
Money is parsed by splitting on the decimal point and concatenating digits —
`Math.round(parseFloat('25.55') * 100)` is rescued by the rounding, but
`25.55 * 100` really is `2554.9999999999995`, and not every case is rescued.
26 tests across the two.

**Form controls are styled globally, not in the `Field` component.** The
control is content-projected, and Angular's emulated encapsulation stamps
projected content with the *parent's* attribute — so a rule written inside
`Field` compiles to a selector its own input can never match. The alternatives
were the deprecated `::ng-deep` or turning encapsulation off, which leaks the
same rules with more ceremony.

**Verified:** build clean, 65 tests pass, and the render-mode split confirmed
against a running SSR server — `/`, `/clubs` and `/seasons` render with content
server-side; every write form ships a bare client shell.

### Phase 5 — account and admin · **done**

- [x] 5.1 Account page — profile, password change, linked providers
- [x] 5.2 Unlink provider, with the last-credential guard
- [x] 5.3 Admin users — list, search, role change, delete
- [x] 5.4 Editor grants per club

**Admin powers are a separate injectable from `AuthRepository`.** Every page
touches `AuthRepository` to read the session; folding "delete any user" into it
would mean the login form injects something that can delete accounts. So
`USER_ADMIN_READER`/`WRITER` are their own contracts — the same
interface-segregation argument as the football repositories (§4), with higher
stakes. It is also the first repository whose base is `api.identity` rather
than `api.football`, which is why `HttpRepository` takes the base as an
abstract member instead of hardcoding one.

**A role change signs that person out everywhere.** The role travels inside the
access token, so the service revokes the target's sessions rather than let a
demoted administrator keep their privileges until the refresh token expired —
up to a day. Correct, and abrupt enough that the confirmation says so, in
different words for your own account than for someone else's.

**Deleting yourself is not offered.** The API refuses it, so the button is
absent on your own row rather than present and guaranteed to fail. Your row is
marked instead.

**Two API gaps recorded rather than worked around**, as N46–N48 in `ISSUES.md`:

- An account created through Google or Facebook **can never obtain a
  password**. Unlinking the last provider is refused with "set a password
  before unlinking it", but `PUT /users/me/password` needs a current password
  the account does not have, and there is no set-password endpoint. The advice
  points at a door that does not exist.
- `GET /users/me` does not report whether a password exists, so the account
  page cannot know in advance whether to offer the form. It offers it to
  everyone and passes the API's (clear) message through. A `has_password` flag
  would fix this.
- `GET /teams/{id}/editors` returns user ids with no way to resolve them —
  identity-svc has no `GET /users/{id}`. The grants screen pages through
  `GET /users` and matches locally, which is fine for a small deployment and
  the same trade `LookupStore` makes for clubs.

### Phase 6 — polish · **done**

- [x] 6.1 Dark mode — three states, with the first paint handled before the stylesheet
- [x] 6.2 Accessibility pass — `aria-describedby` wiring, `scope="col"`, labelled controls
- [x] 6.3 Skeleton loaders
- [x] 6.4 Real 404 page — and a real 404 *status*, which took more than a component
- [x] 6.5 Bundle budgets tightened to 420kB warn / 550kB error (actual: 391kB raw, 108kB transfer)
- [x] 6.6 Dockerfile — a Node SSR runtime, replacing a two-line nginx stub that served nothing
- [x] 6.7 Frontend job in CI, including an assertion that SSR actually rendered
- [x] 6.8 SEO metadata, JSON-LD, canonical URLs and `robots.txt` — see below

**The SEO work was the point of choosing SSR, and none of it existed.** The
page title was the generated `Frontend`, with no description, no Open Graph
tags, no canonical and no structured data. `Seo` now sets all of them from
entity data during the server render, so they are in the first response rather
than appearing after hydration — which is too late for a link unfurler and
unreliable for a crawler. Players and coaches emit `Person`, clubs
`SportsTeam`, competitions `SportsOrganization`.

`SITE_URL` is its own token, deliberately not `API_CONFIG`. One answers "where
do I fetch data from" — inside Docker, an internal hostname no visitor can
reach — and the other "what address is this page published at". The renderer
cannot infer the second from the `Host` header without letting anyone point
this site's canonical tags at theirs, so it is configuration. **Set it to the
real domain in production**, or every canonical link says `localhost`.

**The soft 404 took two attempts, and the first was wrong.** Replacing
`{ path: '**', redirectTo: '' }` with a not-found component fixed the page but
not the status: a wildcard route that resolves to a component is still a
*match*, so the render succeeds and the server answers `200 OK`. The page said
"not found" while the status said otherwise. The fix is `status: 404` on the
catch-all in `app.routes.server.ts` — which only works because every real route
is now enumerated above it, instead of a trailing `**` sweeping up everything.
That has a useful side effect: forgetting to list a new public page makes it
404 loudly rather than fail quietly. CI asserts both directions.

**Both duplicate URLs now name one canonical.** `/` and `/transfers` render the
same component, so they competed as duplicates. They both point at `/` — the
root is the stronger address, and canonicalising it *away* to an alias would
have been worse than leaving it alone.

**Dark mode needs three states, not two.** A boolean cannot express "follow my
system", so anyone who once tapped the toggle would be pinned forever,
including when their OS switches at sunset. The first paint is handled by a
small inline script in `index.html`, ahead of the stylesheet — doing it from
Angular runs after the page has already painted in the system theme, which is
exactly the flash the preference exists to prevent, and worst for the person
who deliberately chose light on a dark machine.

**`Field` was not keeping its own promise.** Its doc comment claimed it owned
the accessibility wiring while rendering `<p id="name-hint">` that nothing
referenced — a screen reader announced the label and then went silent about the
constraint the field was failing. The control is projected so the attribute
cannot be bound in the template; `afterRenderEffect` sets `aria-describedby`
and `aria-invalid` on the element instead, and re-runs as the error appears and
clears.

**Prettier had never been run.** A `.prettierrc` existed and 39 files failed it,
including the ones `ng new` generated. Rather than add a CI step that fails on
arrival, the tree was formatted once. The check is now meaningful.

**Not verified locally:** the container image. Docker is not on the PATH in this
environment, so the CI job is the first thing that will actually build it.

---

## 8. Testing

65 tests across 6 files.

- [x] Unit: `MoneyPipe` (16), money parsing (14), date conversion (12)
- [x] **The concurrent-401 refresh test** (10) — the one bug that will otherwise reach production
- [x] `Permissions` (9), using an in-memory `TeamEditorReader` — the Liskov payoff from §4: swapping the provider, not mocking HTTP
- [ ] Component tests for the forms themselves
- [ ] One end-to-end path: search → player → record transfer → see it in the feed

**Windows note:** the runner occasionally fails with *"Timeout waiting for
worker to respond"* and reports `no tests` — a false red, distinct from the
false green the `forks` pool produced before §1's config change. It passes on
a re-run. Worth knowing before chasing a failure that is not in the code.

---

## 9. Decisions taken

| Decision | Why |
|---|---|
| Standalone components, no NgModules | Angular 21 default; lazy loading per route is simpler |
| Signals over RxJS for state | Less machinery for what is mostly request/response |
| RxJS kept for HTTP and the interceptor | Where its operators genuinely earn their place |
| Vanilla CSS, no framework | Per PROJECT_SPEC |
| Facade per feature | Keeps components free of orchestration |
| Interfaces + InjectionTokens | Testability and transport independence (§4) |
| No chart library | One sparkline does not justify the dependency |
| Access token in memory only | 15-minute life; dies with the tab |

## 10. Answered

**Design — distinct, not a Transfermarkt clone.** Starting direction: an
*almanac*, not a stats terminal. Transfermarkt is dense tables and utilitarian
chrome; this leans editorial — a serif for names and headings, a clean sans for
interface text, monospace for data columns, generous spacing, a restrained
palette. It suits a product whose defining idea is that history is the source
of truth. Revisable once there are real pages to look at; §0.5 only needs the
token set.

**i18n — no.** English only. Not deferred behind an abstraction either: an
unused i18n layer is a cost with no benefit. Note the *data* is multilingual
(Süper Lig, Beşiktaş, Mönchengladbach) — so the UI is English while content is
whatever was entered, and fonts and layout must handle non-Latin text.

**SSR — yes, for SEO.** Public pages must be server-rendered. See §11, because
this changes more than a flag.

---

## 11. What SSR changes

Chosen up front because retrofitting it is painful. Five consequences, none of
them obvious:

### 11.1 `localStorage` does not exist on the server

`SessionStore` must be platform-aware. Reading storage during server rendering
throws, and session restore must run **only in the browser**:

```ts
private readonly isBrowser = isPlatformBrowser(inject(PLATFORM_ID));
```

This is the most common way an SSR app breaks, and the error message points at
the wrong place.

### 11.2 Server-rendered pages are anonymous

The Node renderer has no access token — the browser holds it. Do not try to
forward credentials into SSR.

That is acceptable precisely because **every SEO-relevant read is public**:
players, clubs, competitions, coaches, the transfer feed. The server renders
what a signed-out visitor sees; the browser then hydrates and re-fetches
anything personal.

### 11.3 The API base URL differs by platform

In the browser the gateway is `http://localhost:8000`. From the Node renderer
inside Docker it is `http://gateway:8000` — a different network entirely.

One `API_BASE_URL` token, provided differently per platform. Hardcoding
`localhost` makes SSR fail only in Docker, which is a miserable thing to debug.

### 11.4 Rendering is hybrid, not all-or-nothing

| Routes | Mode | Why |
|---|---|---|
| `/`, `/players/:id`, `/clubs/**`, `/competitions/**`, `/coaches/:id`, `/transfers` | Server-rendered | The SEO surface |
| `/login`, `/register`, `/auth/callback` | Client only | Nothing to index; touches storage |
| `/account/**`, `/admin/**` | Client only | Private, and needs a token |

Angular 21 expresses this in `app.routes.server.ts`.

### 11.5 The container becomes Node, not nginx

The placeholder Dockerfile serves static files with nginx. SSR needs a Node
process running the built server bundle, and the compose port changes from
`4200:80` to `4000:4000`. Step 6.6 covers it.

### Also worth doing for SEO

- [ ] Per-page `<title>` and meta description from the entity
- [ ] Open Graph tags so a shared player link previews properly
- [ ] JSON-LD structured data — `Person` for players, `SportsTeam` for clubs
- [ ] Canonical URLs
- [ ] `sitemap.xml` generated from the API
- [ ] `robots.txt` disallowing `/admin` and `/account`

---

## Progress

| Phase | Status |
|---|---|
| 0 Foundations | **Done** — 2026-08-13 |
| 1 Core plumbing | **Done** — 2026-08-13 |
| 2 Auth | **Done** — 2026-08-15 |
| 3 Read-only app | **Mostly done** — 2026-08-15 |
| 4 Writes | **Done** — 2026-08-18 |
| 5 Account and admin | **Done** — 2026-08-18 |
| 6 Polish | **Done** — 2026-08-18 |

### Carried forward

Real gaps, listed so they are not rediscovered by surprise:

- ~~Competition history is written but never displayed.~~ **Closed** — 2026-08-19.
  The club page has a Competitions section reading `TeamReader.seasons()`, and
  the competition page has a season picker reading `SeasonReader.teams()`.
  Administrators can withdraw an entry, which was a third method with no caller.
  All three existed and were dead before this. See §12.
- **The transfer feed says "Player" on every row.** The landing page's Player
  column is a hardcoded placeholder — `transfer-feed.ts` links to
  `/players/{player_id}` with the literal text `Player`. A transfer carries
  `player_id` and no name, and unlike clubs and competitions, players cannot be
  bulk-cached: there are 45 now and could be a hundred thousand. Fixing it
  properly needs the API to help, either with a batch `GET /players?ids=` or a
  name embedded on the transfer row. A frontend-only fix means up to 25 extra
  requests per page. **This is the most visible defect in the app** — it is on
  the page everyone lands on.
- **A missing entity returns 200.** `/clubs/<unknown-id>` renders "No club with
  that id" with an OK status. `ServerRoute.status` is static per route, so
  fixing this needs the render to influence the response — a real limitation,
  not an oversight.
- **`ng serve` cannot reach the split-service dev mode.** `apiConfigFor` always
  appends `/api/football` and `/api/identity`, which only exist behind the
  gateway. `scripts\dev-run.ps1` runs the services on their own ports with no
  gateway — the documented path on a machine without Docker — and every request
  from the browser 404s. The SSR side now takes `FOOTBALL_API_URL` and
  `IDENTITY_API_URL` overrides; the browser has no runtime environment to read,
  so it needs either a `proxy.conf.json` for `ng serve` or a build-time config.
  That those services set `CORS_ALLOWED_ORIGINS` to the frontend's origin shows
  the mode was always meant to be usable from the app.
- **No `sitemap.xml`.** It has to be generated from the API at request time;
  `robots.txt` already points at it.
- Phase 3's `[~]` items: no type-ahead dropdown, transfer feed filters by type
  only, partial responsive sweep.
- Component tests for the forms, and the end-to-end path in §8.

### Running it

```powershell
cd apps\frontend
npm start                    # dev server on :4200, talks to the gateway on :8000
npm run build                # browser + server bundles
node dist\frontend\server\server.mjs    # SSR on :4000
```

The backend must be up for data to appear — either `docker compose up -d` for
the full stack, or the two `scripts\dev-run.ps1` services. With it down the
page still renders; the competitions section reports that it could not reach
the API.

### What exists so far

```
apps/frontend/src/
  styles.css                        design tokens, form controls, buttons
  server.ts                         Express + SSR, host allowlist
  app/
    app.config.ts / .server.ts      providers, API_CONFIG per platform
    app.routes.ts                   lazy routes and guards
    app.routes.server.ts            render modes
    core/
      tokens/api-config.ts          API_CONFIG, per-platform
      models/                       wire types for both services
      api/
        page.ts  api-error.ts       Page<T>, ApiError with typed codes
        contracts.ts                reader/writer interfaces + tokens
        user-admin.ts               account administration, kept apart
        providers.ts                the only file naming a concrete class
        lookup-store.ts             club and competition names, cached
        http/                       HttpRepository base + implementations
      auth/
        session-store.ts            signals; isAdmin/isEditor computed
        token-storage.ts            access in memory, refresh in localStorage
        auth-interceptor.ts         single-flight refresh
        permissions.ts              the write rules, mirrored from the server
        guards.ts                   navigation only, never security
    shared/
      forms/                        Field, ClubSelect, error mapping
      util/                         date and money conversion
      pipes/  pagination/  ui/      MoneyPipe, Paginator, states, actions
    features/
      transfers/  players/  clubs/  competitions/  coaches/  seasons/
      search/  auth/  account/  admin/
    layout/shell.ts                 masthead, nav, search, account area
```

---

## 12. Competition history — the write with no read

Found by using the app, not by reading it: entering a club in a competition
saved successfully and then nothing anywhere displayed it.

Three repository methods existed, were correct, and had **no caller in the
application**:

| Method | Endpoint | Now used by |
|---|---|---|
| `TeamReader.seasons()` | `GET /teams/{id}/seasons` | club page, Competitions section |
| `SeasonReader.teams()` | `GET /seasons/{id}/teams` | competition page, season picker |
| `TeamWriter.withdrawSeason()` | `DELETE /teams/{id}/seasons/{entryId}` | club page, admin-only remove |

That is the shape of the mistake: the contracts and the HTTP layer were built
from the API surface, the write form was built in phase 4, and the read was
never built at all. A form that saves into a void is worse than no form,
because it looks like it worked.

### Why seasons attach to clubs rather than to competitions

This part was not a bug, and it is worth writing down because it looks like
one. The join is a **triple** — `team_seasons(team_id, season_id, league_id)` —
and one row means *"this club played in this competition in that season."*

A competition has no season list of its own because a competition is permanent:
the Süper Lig is one row that exists across all years. What varies per season is
**who contested it**, so the season-to-competition link is derived from the
entries rather than stored twice. The alternative, a `league_seasons` table,
would either duplicate the club list or contradict it.

### The two views answer different questions

The competition page now has a picker, and the distinction it draws is the
whole point of the temporal model:

- **Now** reads `teams.league_id` — a single pointer to a club's present
  competition. Relegation overwrites it, so it says nothing about the past.
- **A season** reads `team_seasons` — who actually contested the competition
  that year. It survives relegation, because it is history.

They can legitimately disagree, and neither is wrong. One list serving both
would have to pick one and be silently wrong about the other.

### Notes from building it

`LookupStore` gained seasons, and its two near-identical paging loops became
one generic `collect()`. A third copy was the point at which duplicating it
stopped being cheaper than extracting it. The same scaling caveat applies:
fine for tens of seasons and hundreds of clubs, wrong past a few thousand.

Seasons are sorted for the picker by `start_date`, not by label. Labels are
free text, and `2026/27` only sorts correctly beside `2025/26` by accident of
notation — `Apertura 2026` would not.

**A backtick inside an inline template comment broke the build twice.** Writing
`` `teams.league_id` `` in an HTML comment inside a `template:` string
terminates the TypeScript template literal, and the resulting errors point at
"unclosed block" and a stray property name rather than at the quote. Component
templates are template literals; markdown-style code quoting does not belong in
them.

**Verified:** build clean, 65 tests pass, formatting clean, and the changed
pages render server-side without error. **Not verified:** the actual data path
— the backend was down while this was written, so the sections have not been
seen populated with real entries. That is the first thing to check when the
stack is back up.

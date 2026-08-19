import { RenderMode, ServerRoute } from '@angular/ssr';

/**
 * Rendering is hybrid, not all-or-nothing.
 *
 * The generated default was `Prerender` for everything, which is build-time
 * static generation — impossible for `/players/:id`, since the ids live in a
 * database the build has never seen. Server rendering per request is what
 * those pages need.
 *
 * Angular validates that every entry here matches a route in the client
 * router, so a mode can only be declared once its page exists.
 *
 * **Every public route is listed explicitly**, rather than being swept up by a
 * trailing `**`. That is what lets the catch-all mean "no such page" and carry
 * a real 404 — see the note at the bottom. It also makes forgetting to add a
 * new public page a loud failure (it 404s) rather than a silent one.
 *
 * Order matters where a literal and a parameter share a prefix: `clubs/new`
 * has to precede `clubs/:id`, or the form is matched as a club.
 */
export const serverRoutes: ServerRoute[] = [
  // --- the SEO surface, rendered per request ---------------------------
  //
  // Every read behind these is public, so the anonymous render the server
  // produces is exactly the page a visitor sees, which is what should be
  // indexed.
  { path: '', renderMode: RenderMode.Server },
  { path: 'transfers', renderMode: RenderMode.Server },
  { path: 'players/:id', renderMode: RenderMode.Server },
  { path: 'clubs', renderMode: RenderMode.Server },
  { path: 'competitions', renderMode: RenderMode.Server },
  { path: 'coaches/:id', renderMode: RenderMode.Server },
  { path: 'seasons', renderMode: RenderMode.Server },

  // Rendered so the markup is there for a crawler following a link, but the
  // page itself carries `noindex, follow`: the query string is an unbounded
  // URL space, and every result page is a worse landing than the record it
  // points at.
  { path: 'search', renderMode: RenderMode.Server },

  // --- write forms, client only ----------------------------------------
  //
  // Three reasons that all point the same way: nothing here is worth indexing,
  // each sits behind a guard the server cannot evaluate (there is no session
  // during rendering), and each needs a token the Node process does not hold.
  // Server-rendering one would produce a signed-out shell that is immediately
  // discarded — and, worse, a guard redirect decided against an anonymous
  // session.
  { path: 'players/new', renderMode: RenderMode.Client },
  { path: 'players/:id/edit', renderMode: RenderMode.Client },
  { path: 'players/:id/transfer', renderMode: RenderMode.Client },
  { path: 'players/:id/values/new', renderMode: RenderMode.Client },
  { path: 'transfers/:id/edit', renderMode: RenderMode.Client },
  { path: 'clubs/new', renderMode: RenderMode.Client },
  { path: 'clubs/:id/edit', renderMode: RenderMode.Client },
  { path: 'clubs/:id/seasons/new', renderMode: RenderMode.Client },
  { path: 'competitions/new', renderMode: RenderMode.Client },
  { path: 'competitions/:id/edit', renderMode: RenderMode.Client },
  { path: 'seasons/new', renderMode: RenderMode.Client },
  { path: 'seasons/:id/edit', renderMode: RenderMode.Client },
  { path: 'coaches/new', renderMode: RenderMode.Client },
  { path: 'coaches/:id/edit', renderMode: RenderMode.Client },
  { path: 'coaches/:id/spells/new', renderMode: RenderMode.Client },

  // Declared after the literal-prefixed write routes above, so `clubs/new` is
  // not matched as a club whose id is "new".
  { path: 'clubs/:id', renderMode: RenderMode.Server },
  { path: 'competitions/:id', renderMode: RenderMode.Server },

  // --- private, client only --------------------------------------------
  { path: 'login', renderMode: RenderMode.Client },
  { path: 'register', renderMode: RenderMode.Client },
  { path: 'auth/callback', renderMode: RenderMode.Client },
  { path: 'account', renderMode: RenderMode.Client },
  { path: 'admin/users', renderMode: RenderMode.Client },
  { path: 'admin/clubs/:id/editors', renderMode: RenderMode.Client },

  // --- anything else is genuinely not a page ---------------------------
  //
  // Rendered, so the visitor gets the not-found page rather than a blank
  // response — but with a real 404 status. Without this the app answers
  // `200 OK` for every mistyped URL, which is a soft 404: the client router
  // matches its own `**` route, so from the server's point of view the render
  // succeeded. A crawler reads that as "every URL on this host is a valid
  // page" and the duplicates are attributed somewhere they should not be.
  { path: '**', renderMode: RenderMode.Server, status: 404 },
];

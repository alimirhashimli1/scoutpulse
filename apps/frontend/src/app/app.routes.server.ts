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
 * router, so a mode can only be declared once its page exists. Add the entry
 * in the same change as the component, not before.
 *
 * The intended split, from docs/FRONTEND.md §11.4:
 *
 *   RenderMode.Client   login, register, auth/callback, search,
 *                       account/**, admin/**
 *
 *     Nothing worth indexing, and all of it either touches localStorage or
 *     needs a token the server does not have. Rendering it server-side would
 *     produce a signed-out shell that flashes and replaces itself.
 *
 *   RenderMode.Server   everything else — the SEO surface
 *
 *     Players, clubs, competitions, coaches, the transfer feed. Every read
 *     behind them is public, so the anonymous render the server produces is
 *     exactly the page a visitor sees, which is what should be indexed.
 */
export const serverRoutes: ServerRoute[] = [
  // Client only: nothing worth indexing, and each touches localStorage or
  // needs a token the server does not have. Rendering them server-side would
  // produce a signed-out shell that flashes and replaces itself.
  { path: 'login', renderMode: RenderMode.Client },
  { path: 'register', renderMode: RenderMode.Client },
  { path: 'auth/callback', renderMode: RenderMode.Client },

  // Every write form, for three reasons that all point the same way: nothing
  // here is worth indexing, each sits behind a guard the server cannot
  // evaluate (there is no session during rendering), and each needs a token
  // the Node process does not hold. Server-rendering one would produce a
  // signed-out shell that is immediately discarded, and — worse — a guard
  // redirect decided against an anonymous session.
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

  // Private by definition. Nothing here should ever be indexed, and the
  // renderer holds no token to fetch any of it with.
  { path: 'account', renderMode: RenderMode.Client },
  { path: 'admin/users', renderMode: RenderMode.Client },
  { path: 'admin/clubs/:id/editors', renderMode: RenderMode.Client },

  // Everything else is the SEO surface, rendered per request. `/seasons`
  // stays here: the list is public data, and only its write controls are not.
  { path: '**', renderMode: RenderMode.Server },
];

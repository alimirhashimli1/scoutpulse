import { Routes } from '@angular/router';

import { adminGuard, authGuard, editorGuard } from './core/auth/guards';

/**
 * Every feature route is lazily loaded, so the initial bundle carries the shell
 * and nothing else. Render modes are set separately in app.routes.server.ts.
 *
 * Route parameters arrive as component inputs via withComponentInputBinding(),
 * which is why the detail components declare `id = input.required<string>()`
 * rather than injecting ActivatedRoute.
 *
 * **Order matters where a literal and a parameter share a prefix.** Angular
 * matches top to bottom, so `clubs/new` has to be declared before `clubs/:id`
 * or the form would be loaded as a club whose id is the word "new" — a 404 on
 * a page that ought to work.
 *
 * The guards are for navigation only. Every rule is enforced independently by
 * the API, and a guard just avoids showing a page that would immediately 403.
 */
export const routes: Routes = [
  {
    path: '',
    loadComponent: () => import('./features/transfers/transfer-feed').then((m) => m.TransferFeed),
    title: 'Transfers · ScoutPulse',
  },
  {
    path: 'transfers',
    loadComponent: () => import('./features/transfers/transfer-feed').then((m) => m.TransferFeed),
    title: 'Transfers · ScoutPulse',
  },
  {
    path: 'search',
    loadComponent: () => import('./features/search/search-page').then((m) => m.SearchPage),
    title: 'Search · ScoutPulse',
  },
  {
    path: 'transfers/:id/edit',
    loadComponent: () => import('./features/transfers/transfer-form').then((m) => m.TransferEdit),
    canActivate: [editorGuard],
    title: 'Correct a transfer · ScoutPulse',
  },

  // --- players ---
  {
    path: 'players/new',
    loadComponent: () => import('./features/players/player-form').then((m) => m.PlayerForm),
    canActivate: [editorGuard],
    title: 'New player · ScoutPulse',
  },
  {
    path: 'players/:id',
    loadComponent: () => import('./features/players/player-page').then((m) => m.PlayerPage),
    title: 'Player · ScoutPulse',
  },
  {
    path: 'players/:id/edit',
    loadComponent: () => import('./features/players/player-form').then((m) => m.PlayerForm),
    canActivate: [editorGuard],
    title: 'Edit player · ScoutPulse',
  },
  {
    // The only route that moves a player. There is no club field anywhere else.
    path: 'players/:id/transfer',
    loadComponent: () => import('./features/transfers/transfer-form').then((m) => m.TransferForm),
    canActivate: [editorGuard],
    title: 'Record a transfer · ScoutPulse',
  },
  {
    path: 'players/:id/values/new',
    loadComponent: () => import('./features/players/value-form').then((m) => m.ValueForm),
    canActivate: [adminGuard],
    title: 'Record a valuation · ScoutPulse',
  },

  // --- clubs ---
  {
    path: 'clubs',
    loadComponent: () => import('./features/clubs/club-list').then((m) => m.ClubList),
    title: 'Clubs · ScoutPulse',
  },
  {
    path: 'clubs/new',
    loadComponent: () => import('./features/clubs/club-form').then((m) => m.ClubForm),
    canActivate: [adminGuard],
    title: 'New club · ScoutPulse',
  },
  {
    path: 'clubs/:id',
    loadComponent: () => import('./features/clubs/club-page').then((m) => m.ClubPage),
    title: 'Club · ScoutPulse',
  },
  {
    path: 'clubs/:id/edit',
    loadComponent: () => import('./features/clubs/club-form').then((m) => m.ClubForm),
    canActivate: [adminGuard],
    title: 'Edit club · ScoutPulse',
  },
  {
    // An editor's write, unlike the club record itself.
    path: 'clubs/:id/seasons/new',
    loadComponent: () =>
      import('./features/clubs/season-entry-form').then((m) => m.SeasonEntryForm),
    canActivate: [editorGuard],
    title: 'Enter a competition · ScoutPulse',
  },

  // --- competitions ---
  {
    path: 'competitions',
    loadComponent: () =>
      import('./features/competitions/competition-pages').then((m) => m.CompetitionList),
    title: 'Competitions · ScoutPulse',
  },
  {
    path: 'competitions/new',
    loadComponent: () =>
      import('./features/competitions/competition-form').then((m) => m.CompetitionForm),
    canActivate: [adminGuard],
    title: 'New competition · ScoutPulse',
  },
  {
    path: 'competitions/:id',
    loadComponent: () =>
      import('./features/competitions/competition-pages').then((m) => m.CompetitionPage),
    title: 'Competition · ScoutPulse',
  },
  {
    path: 'competitions/:id/edit',
    loadComponent: () =>
      import('./features/competitions/competition-form').then((m) => m.CompetitionForm),
    canActivate: [adminGuard],
    title: 'Edit competition · ScoutPulse',
  },

  // --- seasons ---
  {
    path: 'seasons',
    loadComponent: () => import('./features/seasons/season-pages').then((m) => m.SeasonList),
    title: 'Seasons · ScoutPulse',
  },
  {
    path: 'seasons/new',
    loadComponent: () => import('./features/seasons/season-pages').then((m) => m.SeasonForm),
    canActivate: [adminGuard],
    title: 'New season · ScoutPulse',
  },
  {
    path: 'seasons/:id/edit',
    loadComponent: () => import('./features/seasons/season-pages').then((m) => m.SeasonForm),
    canActivate: [adminGuard],
    title: 'Edit season · ScoutPulse',
  },

  // --- coaches ---
  {
    path: 'coaches/new',
    loadComponent: () => import('./features/coaches/coach-form').then((m) => m.CoachForm),
    canActivate: [editorGuard],
    title: 'New coach · ScoutPulse',
  },
  {
    path: 'coaches/:id',
    loadComponent: () => import('./features/coaches/coach-page').then((m) => m.CoachPage),
    title: 'Coach · ScoutPulse',
  },
  {
    path: 'coaches/:id/edit',
    loadComponent: () => import('./features/coaches/coach-form').then((m) => m.CoachForm),
    canActivate: [editorGuard],
    title: 'Edit coach · ScoutPulse',
  },
  {
    path: 'coaches/:id/spells/new',
    loadComponent: () => import('./features/coaches/spell-form').then((m) => m.SpellForm),
    canActivate: [editorGuard],
    title: 'Record an appointment · ScoutPulse',
  },
  {
    path: 'login',
    loadComponent: () => import('./features/auth/login').then((m) => m.Login),
    title: 'Sign in · ScoutPulse',
  },
  {
    path: 'register',
    loadComponent: () => import('./features/auth/register').then((m) => m.Register),
    title: 'Create an account · ScoutPulse',
  },
  {
    path: 'auth/callback',
    loadComponent: () => import('./features/auth/oauth-callback').then((m) => m.OAuthCallback),
    title: 'Signing in · ScoutPulse',
  },

  // --- account and administration ---
  {
    path: 'account',
    loadComponent: () => import('./features/account/account-page').then((m) => m.AccountPage),
    canActivate: [authGuard],
    title: 'Account · ScoutPulse',
  },
  {
    path: 'admin/users',
    loadComponent: () => import('./features/admin/user-list').then((m) => m.UserList),
    canActivate: [adminGuard],
    title: 'Users · ScoutPulse',
  },
  {
    path: 'admin/clubs/:id/editors',
    loadComponent: () => import('./features/admin/club-editors').then((m) => m.ClubEditors),
    canActivate: [adminGuard],
    title: 'Editor access · ScoutPulse',
  },
  // A real 404, not a redirect to the home page. Redirecting answered 200 OK
  // for every dead link, which loses the address a visitor followed and lets a
  // crawler treat an unbounded set of non-existent URLs as valid pages.
  {
    path: '**',
    loadComponent: () => import('./features/errors/not-found').then((m) => m.NotFound),
    title: 'Page not found · ScoutPulse',
  },
];

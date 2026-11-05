import { Routes } from '@angular/router';

/**
 * The games section, loaded as one lazy child tree. Each game is its own
 * chunk, so the hub does not carry any game's code.
 */
export const GAMES_ROUTES: Routes = [
  {
    path: '',
    loadComponent: () => import('./hub/games-hub.component').then((m) => m.GamesHub),
    title: 'Games · ScoutPulse',
  },
  {
    path: 'played-for-both',
    loadComponent: () =>
      import('./played-for-both/played-for-both.page').then((m) => m.PlayedForBothPage),
    title: 'Who Played For Both? · ScoutPulse',
  },
];

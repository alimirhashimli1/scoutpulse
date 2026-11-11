import { Routes } from '@angular/router';

import { GAME_ROOMS } from '../../core/api/games';
import { HttpGameRooms } from '../../core/api/http/game-room-repository';

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
    // Here rather than with the app-wide bindings: only this game goes
    // online, so the client loads with it.
    providers: [HttpGameRooms, { provide: GAME_ROOMS, useExisting: HttpGameRooms }],
  },
];

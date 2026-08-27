import { Provider } from '@angular/core';

import {
  COACH_READER,
  COACH_WRITER,
  LEAGUE_READER,
  LEAGUE_WRITER,
  PLAYER_NOTE_READER,
  PLAYER_NOTE_WRITER,
  PLAYER_READER,
  PLAYER_WRITER,
  SEARCH_READER,
  SEASON_READER,
  SEASON_WRITER,
  TEAM_EDITOR_READER,
  TEAM_EDITOR_WRITER,
  TEAM_READER,
  TEAM_WRITER,
  TRANSFER_READER,
  TRANSFER_WRITER,
} from './contracts';
import {
  HttpCoachRepository,
  HttpLeagueRepository,
  HttpPlayerRepository,
  HttpSearchRepository,
  HttpSeasonRepository,
  HttpTeamEditorRepository,
  HttpTeamRepository,
  HttpTransferRepository,
} from './http/football-repositories';
import { HttpUserAdminRepository } from './http/identity-repositories';
import { USER_ADMIN_READER, USER_ADMIN_WRITER } from './user-admin';

/**
 * Binds every contract to its HTTP implementation.
 *
 * This file is the *only* place the concrete classes are named. Everything
 * else injects a token, so replacing the transport — an in-memory repository
 * in a test, a different backend later — is a change here and nowhere else.
 *
 * Reader and writer tokens resolve to the same instance via `useExisting`, so
 * a page holding both does not open two of anything. The split exists to
 * constrain what a consumer can call, not to create two objects.
 */
export function provideFootballApi(): Provider[] {
  return [
    HttpPlayerRepository,
    { provide: PLAYER_READER, useExisting: HttpPlayerRepository },
    { provide: PLAYER_WRITER, useExisting: HttpPlayerRepository },
    { provide: PLAYER_NOTE_READER, useExisting: HttpPlayerRepository },
    { provide: PLAYER_NOTE_WRITER, useExisting: HttpPlayerRepository },

    HttpTeamRepository,
    { provide: TEAM_READER, useExisting: HttpTeamRepository },
    { provide: TEAM_WRITER, useExisting: HttpTeamRepository },

    HttpLeagueRepository,
    { provide: LEAGUE_READER, useExisting: HttpLeagueRepository },
    { provide: LEAGUE_WRITER, useExisting: HttpLeagueRepository },

    HttpSeasonRepository,
    { provide: SEASON_READER, useExisting: HttpSeasonRepository },
    { provide: SEASON_WRITER, useExisting: HttpSeasonRepository },

    HttpCoachRepository,
    { provide: COACH_READER, useExisting: HttpCoachRepository },
    { provide: COACH_WRITER, useExisting: HttpCoachRepository },

    HttpTransferRepository,
    { provide: TRANSFER_READER, useExisting: HttpTransferRepository },
    { provide: TRANSFER_WRITER, useExisting: HttpTransferRepository },

    HttpTeamEditorRepository,
    { provide: TEAM_EDITOR_READER, useExisting: HttpTeamEditorRepository },
    { provide: TEAM_EDITOR_WRITER, useExisting: HttpTeamEditorRepository },

    HttpSearchRepository,
    { provide: SEARCH_READER, useExisting: HttpSearchRepository },

    // identity-svc, not football-svc. Kept in the same call because it is the
    // same decision — which class answers which contract — and splitting it
    // would only mean two functions to remember to call.
    HttpUserAdminRepository,
    { provide: USER_ADMIN_READER, useExisting: HttpUserAdminRepository },
    { provide: USER_ADMIN_WRITER, useExisting: HttpUserAdminRepository },
  ];
}

import { InjectionToken } from '@angular/core';

import { GameSummary, PlayedForBothWire } from '../models/games';

/**
 * The games section. Read only: every game runs in the browser and the
 * service just hands over the list and each game's data.
 */
export interface GameReader {
  /** Every game the hub shows, in order. */
  list(): Promise<GameSummary[]>;
  /**
   * The whole "Who Played For Both?" dataset (~430 KB). Fetched once per
   * session; later calls return the same promise.
   */
  playedForBothDataset(): Promise<PlayedForBothWire>;
}

export const GAME_READER = new InjectionToken<GameReader>('GameReader');

import { InjectionToken } from '@angular/core';

import {
  GameSummary,
  GuessResult,
  PlayedForBothWire,
  RoomSeat,
  RoomSettings,
  RoomSnapshot,
} from '../models/games';

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

/**
 * Online rooms of "Who Played For Both?". The server runs the game; these
 * calls act on it, and `eventsUrl` is the stream it pushes the room down.
 */
export interface GameRooms {
  create(name: string): Promise<RoomSeat>;
  join(code: string, name: string): Promise<RoomSeat>;
  configure(seat: RoomSeat, settings: RoomSettings): Promise<void>;
  start(seat: RoomSeat): Promise<void>;
  next(seat: RoomSeat): Promise<void>;
  stop(seat: RoomSeat): Promise<void>;
  guess(seat: RoomSeat, text: string): Promise<GuessResult>;
  /** Leaves; also works while the page is closing. */
  leave(seat: RoomSeat): void;
  /** The room once, for when the stream does not get through. */
  poll(seat: RoomSeat): Promise<RoomSnapshot>;
  /** For an EventSource. */
  eventsUrl(seat: RoomSeat): string;
}

export const GAME_ROOMS = new InjectionToken<GameRooms>('GameRooms');

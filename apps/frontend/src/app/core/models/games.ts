/**
 * Browser games. The hub list and each game's static data, as served by
 * football-svc under /api/v1/games.
 */

/** One card on the games hub. Title and description are i18n keys. */
export interface GameSummary {
  slug: string;
  titleKey: string;
  descriptionKey: string;
  players: number;
  status: 'live' | 'soon';
}

/**
 * The "Who Played For Both?" dataset exactly as it comes over the wire.
 *
 * Positional arrays rather than objects: ~11,000 players as objects would be
 * roughly twice the size. `parseDataset` in the game's engine gives each
 * position a name once, on load.
 */
export interface PlayedForBothWire {
  /** [code, country, leagueName] */
  leagues: [string, string, string][];
  /** [id, name, colour1, colour2, leagueIndex] */
  clubs: [string, string, string, string, number][];
  /** [fifaCode, nameEN, nameTR, flagIconsCode] */
  nations: [string, string, string, string][];
  /** [name, nationIndex (-1 = none), clubIndexes, famous 0|1] */
  players: [string, number, number[], number][];
  /** Difficulty levels. Absent from a copy cached before they existed. */
  levels?: LevelWire[];
}

/** The difficulty levels, defined by the server; see playedforboth/levels.go. */
export type LevelCode = 'easy' | 'medium' | 'hard';

/** One difficulty level. A null or empty list means "no limit". */
export interface LevelWire {
  code: LevelCode;
  /** League codes in play. */
  leagues: string[] | null;
  famousOnly: boolean;
  /** When set, only these club ids are in play (the big clubs of easy). */
  clubs: string[] | null;
  /** FIFA codes of the nations that may stand on one side. */
  nations: string[] | null;
  /** How often a club in play meets a smaller club from widerLeagues. */
  widerChance: number;
  widerLeagues: string[] | null;
}

// --- online rooms ------------------------------------------------------------

/** A seat in an online room: the token proves it on every call. */
export interface RoomSeat {
  code: string;
  playerId: string;
  token: string;
}

export type RoomPhase = 'lobby' | 'playing' | 'reveal' | 'finished';

export interface RoomSettings {
  level: LevelCode;
  /** Seconds a pair stays up. */
  timer: 20 | 30 | 60;
  /** Points that win; 0 plays on. */
  target: 0 | 3 | 5 | 10;
}

export interface RoomPlayer {
  id: string;
  name: string;
  score: number;
  online: boolean;
}

/** The whole room, as the server pushes it after every change. */
export interface RoomSnapshot {
  code: string;
  version: number;
  phase: RoomPhase;
  round: number;
  host: string;
  players: RoomPlayer[];
  settings: RoomSettings;
  /** The pair by entity index (clubs, then nations), and its clock. */
  pair: { left: number; right: number; remainingMs: number; durationMs: number } | null;
  /** Who took the round and with which name; null when nobody did. */
  win: { player: string; answer: string } | null;
  /** The game's winner, once the target is reached. */
  champion: string;
}

export interface GuessResult {
  correct: boolean;
  /** Right, but someone was quicker. */
  late?: boolean;
}

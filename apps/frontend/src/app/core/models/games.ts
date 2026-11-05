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
}

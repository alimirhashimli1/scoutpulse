import { Injectable, computed, signal } from '@angular/core';

/**
 * The games section's strings.
 *
 * The games once offered English, Turkish and German; they are English only
 * now, like the rest of ScoutPulse. The strings stay in one dictionary behind
 * this small service so the components need not change if a language is ever
 * added back.
 */

export const GAME_LANGS = ['en'] as const;
export type GameLang = (typeof GAME_LANGS)[number];

/** A club's name as the dataset has it. */
export function clubName(club: { id: string; name: string }, _lang: GameLang): string {
  return club.name;
}

/** A nation's English name. */
export function nationName(nation: { nameEN: string }, _lang: GameLang): string {
  return nation.nameEN;
}

export interface NoteCounts {
  players: number;
  clubs: number;
  nations: number;
  leagues: number;
}

const en = {
  // --- hub
  gamesTitle: 'Games',
  gamesLede: 'Football trivia to play with a friend on one screen.',
  playersTag: (n: number) => `${n} players`,
  play: 'Play',
  soon: 'Coming soon',
  backToGames: 'All games',
  hubError: 'The games could not be loaded.',
  datasetError: 'The game data could not be loaded.',
  retry: 'Retry',
  loading: 'Loading the game…',
  language: 'Language',
  'games.playedForBoth.title': 'Who Played For Both?',
  'games.playedForBoth.description':
    'Name a footballer who played for both teams. Play on one screen, or online against friends.',

  // --- game (from the reference)
  title: 'Who played for both?',
  sub: 'Name a footballer who played for the club on the left and the club or country on the right. First correct answer wins the point.',
  reveal: 'Reveal answer',
  next: 'Next pair',
  start: 'Start game',
  idle: 'Press Start for the first pair',
  mystery: 'Mystery team',
  space: 'Space',
  fit: 'Players who fit',
  settings: 'Game settings',
  optKnown:
    'Only show pairs with at least one known answer. Turn off for fully random pairs, which can be impossible.',
  optNation: 'Allow countries (club vs country). Turn off for club vs club only.',
  optFamous:
    'Build pairs from well-known players only. Turn off for a much harder game with lesser-known names.',
  leaguesLbl: 'Leagues in play',
  allL: 'All leagues',
  top5: 'Top 5 only',
  noneL: 'Clear',
  timer: 'Round timer',
  off: 'Off',
  s20: '20 s',
  s30: '30 s',
  s60: '60 s',
  reset: 'Reset scores',
  scoreboard: 'Scoreboard',
  timeLeft: 'Time left',
  round: (n: number) => `Round ${n}`,
  player: (n: number) => `Player ${n}`,
  playerName: (n: number) => `Player ${n} name`,
  point: (name: string) => `+1 ${name}`,
  club: 'Club',
  nation: 'National team',
  flagOf: (n: string) => `Flag of ${n}`,
  badgeOf: (n: string) => `${n} badge`,
  country: (c: string) => c,
  more: (n: number) => `+${n} more`,
  needLeague: 'Pick at least one league to play.',

  // --- modes and difficulty
  modes: 'How to play',
  modeLocal: 'Same screen',
  modeOnline: 'Online',
  difficulty: 'Difficulty',
  easy: 'Easy',
  medium: 'Medium',
  hard: 'Hard',
  custom: 'Custom',
  easyHint: 'Big clubs and top national teams, now and then a smaller club.',
  mediumHint: "Clubs from Europe's eight main leagues, well-known players.",
  hardHint: 'Every league, and lesser-known players too.',
  customHint: 'Your own leagues and options, under Game settings.',

  // --- online
  onlineIntro:
    'Play against friends on their own phones or computers. Whoever first types a right name, first name or surname, takes the point.',
  yourName: 'Your name',
  createRoom: 'Create a game',
  orJoin: 'or join one',
  roomCode: 'Game code',
  join: 'Join',
  shareHint: 'Send the code or the link to the others.',
  copyLink: 'Copy link',
  copied: 'Copied',
  leave: 'Leave game',
  players: 'Players',
  you: 'you',
  hostTag: 'host',
  offline: 'offline',
  waitingHost: (n: string) => `Waiting for ${n} to start the game.`,
  needTwo: 'Waiting for at least one more player…',
  firstTo: 'First to',
  pointsN: (n: number) => `${n} points`,
  noLimit: 'No limit',
  hostOnly: 'Only the host can change these.',
  guessLabel: 'Name a player who played for both',
  guessPlaceholder: 'First name or surname, then Enter',
  send: 'Send',
  wrong: 'Not that one, keep going',
  late: 'Right, but someone was quicker',
  gotIt: (who: string, answer: string) => `${who} got it: ${answer}`,
  youGotIt: (answer: string) => `You got it: ${answer}`,
  nobody: 'Nobody got it.',
  nextSoon: 'Next pair in a moment…',
  skip: 'Skip pair',
  nextNow: 'Next pair now',
  endGame: 'End game',
  champion: (n: string) => `${n} wins the game!`,
  youWin: 'You win the game!',
  playAgain: 'Play again',
  reconnecting: 'Connection lost, reconnecting…',
  roomGone: 'That game has ended. Start a new one, or join another.',
  roomError: 'Something went wrong. Try again.',
  levelSummary: (level: string, timer: number, target: string) =>
    `${level} · ${timer} s a pair · ${target}`,
  none: (a: string, b: string) =>
    `No player in this game's list played for both ${a} and ${b}. If someone named a real one, give them the point.`,
  note: (c: NoteCounts) =>
    `Country flags are real. Clubs are shown as badges in club colours rather than official crests, which are trademarks. The game knows ${c.players.toLocaleString('en')} players across ${c.clubs} clubs in ${c.leagues} leagues and ${c.nations} national teams. Careers come from historic transfer records (1992–2022), big-five league squads (2010–2026) and hand-checked legends, and include loan spells. The other player may still know a correct name that isn't listed.`,
  sources:
    'Sources: Transfermarkt transfer records 1992–2022 (github.com/ewenme/transfers), Transfermarkt player profiles (github.com/salimt/football-datasets), FBref big-five league squads 2010–2026 (worldfootballR_data), plus hand-checked legends. Flags: flag-icons (MIT).',
};

export type GameStrings = typeof en;

export const GAME_STRINGS: Readonly<Record<GameLang, GameStrings>> = { en };

@Injectable({ providedIn: 'root' })
export class GamesI18n {
  readonly lang = signal<GameLang>('en').asReadonly();
  readonly t = computed(() => GAME_STRINGS[this.lang()]);

  /** A catalog key from the server, or the key itself if this build lacks it. */
  catalog(key: string): string {
    const value = (this.t() as unknown as Record<string, unknown>)[key];
    return typeof value === 'string' ? value : key;
  }
}

import { LevelCode, LevelWire, PlayedForBothWire } from '../../../core/models/games';

/**
 * The rules of "Who Played For Both?", as pure functions.
 *
 * Nothing here touches Angular, the DOM or `Math.random` directly: randomness
 * comes in as an `Rng`, so the specs can draw thousands of pairs
 * deterministically and the store stays a thin layer of signals on top.
 *
 * **Entities.** A club or a national team, in one numeric space: clubs are
 * `0 .. C-1` in dataset order and nations follow at `C .. C+N-1`. A pair is
 * kept as one number, `min * PAIR_BASE + max`, so the pair pool and the
 * recent-pairs window are plain number arrays and sets.
 */

export interface League {
  code: string;
  country: string;
  name: string;
}

export interface Club {
  id: string;
  name: string;
  colour1: string;
  colour2: string;
  league: number;
}

export interface Nation {
  fifa: string;
  nameEN: string;
  nameTR: string;
  flag: string;
}

export interface Player {
  name: string;
  /** Club entities, then the nation entity when there is one. */
  entities: number[];
  famous: boolean;
}

export interface GameIndex {
  leagues: League[];
  clubs: Club[];
  nations: Nation[];
  players: Player[];
  /** Number of clubs; the first nation's entity index. */
  clubCount: number;
  /** Entity → indexes into `players`. Built once on load. */
  byEntity: number[][];
  /** League index → how many clubs it has, for the settings panel. */
  clubsPerLeague: number[];
  /** The difficulty levels, easiest first; empty from an old cached file. */
  levels: LevelWire[];
}

export type Rng = () => number;

/** Wide enough for any entity index; keeps pair keys exact integers. */
export const PAIR_BASE = 100_000;
/** How many recent pairs may not come up again. */
export const RECENT_WINDOW = 60;
/** How hard to look for a pair outside that window before accepting a repeat. */
export const MAX_TRIES = 50;
/** In fully random mode, how often the second side is a country. */
export const NATION_CHANCE = 0.35;
/** The "Top 5 only" preset. */
export const TOP5: readonly string[] = ['eng', 'esp', 'ita', 'ger', 'fra'];

// --- loading ----------------------------------------------------------------

export function buildIndex(wire: PlayedForBothWire): GameIndex {
  const leagues = wire.leagues.map(([code, country, name]) => ({ code, country, name }));
  const clubs = wire.clubs.map(([id, name, colour1, colour2, league]) => ({
    id,
    name,
    colour1,
    colour2,
    league,
  }));
  const nations = wire.nations.map(([fifa, nameEN, nameTR, flag]) => ({
    fifa,
    nameEN,
    nameTR,
    flag,
  }));
  const clubCount = clubs.length;

  const players: Player[] = wire.players.map(([name, nation, clubIdx, famous]) => ({
    name,
    entities: nation >= 0 ? [...clubIdx, clubCount + nation] : clubIdx.slice(),
    famous: famous === 1,
  }));

  const byEntity: number[][] = Array.from({ length: clubCount + nations.length }, () => []);
  players.forEach((p, i) => {
    for (const e of p.entities) byEntity[e].push(i);
  });

  const clubsPerLeague = leagues.map(() => 0);
  for (const c of clubs) clubsPerLeague[c.league]++;

  const levels = wire.levels ?? [];
  return { leagues, clubs, nations, players, clubCount, byEntity, clubsPerLeague, levels };
}

// --- entities and pairs -------------------------------------------------------

export const isClub = (ix: GameIndex, e: number): boolean => e < ix.clubCount;

export const nationEntity = (ix: GameIndex, nationIndex: number): number =>
  ix.clubCount + nationIndex;

export function pairKey(a: number, b: number): number {
  return Math.min(a, b) * PAIR_BASE + Math.max(a, b);
}

export function splitKey(key: number): [number, number] {
  return [Math.floor(key / PAIR_BASE), key % PAIR_BASE];
}

/** Players linked to both entities: famous first, then alphabetical. */
export function matches(ix: GameIndex, a: number, b: number): Player[] {
  const other = new Set(ix.byEntity[b]);
  return ix.byEntity[a]
    .filter((i) => other.has(i))
    .map((i) => ix.players[i])
    .sort((x, y) => Number(y.famous) - Number(x.famous) || x.name.localeCompare(y.name));
}

/** Club entities whose league is switched on, in dataset order. */
export function enabledClubs(ix: GameIndex, leagues: ReadonlySet<string>): number[] {
  const out: number[] = [];
  ix.clubs.forEach((c, i) => {
    if (leagues.has(ix.leagues[c.league].code)) out.push(i);
  });
  return out;
}

export interface PoolOptions {
  leagues: ReadonlySet<string>;
  famousOnly: boolean;
  allowNations: boolean;
}

/**
 * Every pair with at least one known answer under these options.
 *
 * Built from the players' side: for each (famous) player, every pair among
 * their enabled clubs, plus each of those clubs with their nation. A pair is
 * therefore never nation–nation and never one entity twice, and `matches()`
 * is non-empty for every key by construction.
 */
export function buildPairPool(ix: GameIndex, opts: PoolOptions): number[] {
  const on = new Set(enabledClubs(ix, opts.leagues));
  const set = new Set<number>();
  for (const p of ix.players) {
    if (opts.famousOnly && !p.famous) continue;
    const clubs = p.entities.filter((e) => on.has(e));
    const nation = p.entities.find((e) => !isClub(ix, e));
    for (let i = 0; i < clubs.length; i++) {
      for (let j = i + 1; j < clubs.length; j++) {
        if (clubs[i] !== clubs[j]) set.add(pairKey(clubs[i], clubs[j]));
      }
      if (opts.allowNations && nation !== undefined) set.add(pairKey(clubs[i], nation));
    }
  }
  return [...set];
}

/** Every pair a difficulty level can draw, each with a known answer. */
export interface LevelPool {
  /** Pairs of teams the level puts in play. */
  core: number[];
  /** A club in play against a smaller club; drawn `widerChance` of the time. */
  wider: number[];
  widerChance: number;
}

/**
 * The pairs of a level. The same rules as BuildPool in the service
 * (playedforboth/game.go), so a same-screen game and an online room agree on
 * what each level means: built from the players' side, never nation against
 * nation, never one team twice.
 */
export function buildLevelPool(ix: GameIndex, level: LevelWire): LevelPool {
  const has = (list: string[] | null, v: string) => !list?.length || list.includes(v);
  const leagueOf = (c: number) => ix.leagues[ix.clubs[c].league].code;
  const inPlay = ix.clubs.map((c, i) =>
    level.clubs?.length ? level.clubs.includes(c.id) : has(level.leagues, leagueOf(i)),
  );
  const wider = ix.clubs.map(
    (_, i) => level.widerChance > 0 && !inPlay[i] && has(level.widerLeagues, leagueOf(i)),
  );
  const nationOk = (e: number | undefined) =>
    e !== undefined && has(level.nations, ix.nations[e - ix.clubCount].fifa);

  const core = new Set<number>();
  const wide = new Set<number>();
  for (const p of ix.players) {
    if (level.famousOnly && !p.famous) continue;
    const mine = p.entities.filter((e) => isClub(ix, e) && inPlay[e]);
    const small = p.entities.filter((e) => isClub(ix, e) && wider[e]);
    const nation = p.entities.find((e) => !isClub(ix, e));
    for (let i = 0; i < mine.length; i++) {
      for (let j = i + 1; j < mine.length; j++) {
        if (mine[i] !== mine[j]) core.add(pairKey(mine[i], mine[j]));
      }
      if (nationOk(nation)) core.add(pairKey(mine[i], nation!));
      for (const b of small) wide.add(pairKey(mine[i], b));
    }
  }
  return { core: [...core], wider: [...wide], widerChance: level.widerChance };
}

/** A level by its code, if the dataset has it. */
export function findLevel(ix: GameIndex, code: LevelCode): LevelWire | undefined {
  return ix.levels.find((l) => l.code === code);
}

/** One pair from a level, avoiding the recent ones as drawPair does. */
export function drawFromLevel(
  pool: LevelPool,
  recent: readonly number[],
  rng: Rng = Math.random,
): Draw | null {
  if (!pool.core.length && !pool.wider.length) return null;
  const seen = new Set(recent);
  let key = 0;
  for (let tries = 0; tries < MAX_TRIES; tries++) {
    const list =
      pool.wider.length && (!pool.core.length || rng() < pool.widerChance) ? pool.wider : pool.core;
    key = pick(list, rng);
    if (!seen.has(key)) break;
  }
  let [left, right] = splitKey(key);
  if (rng() < 0.5) [left, right] = [right, left];
  return { key, left, right };
}

export interface DrawOptions {
  /** Enabled clubs, from `enabledClubs`. */
  clubs: readonly number[];
  /** The known-answer pool, from `buildPairPool`; used when `knownOnly`. */
  pool: readonly number[];
  knownOnly: boolean;
  allowNations: boolean;
  /** Keys of the most recent pairs, oldest first. */
  recent: readonly number[];
}

export interface Draw {
  key: number;
  /** As shown: which entity is on which side is a coin toss. */
  left: number;
  right: number;
}

const pick = <T>(list: readonly T[], rng: Rng): T => list[Math.floor(rng() * list.length)];

/**
 * One random pair to show, or null when no league is switched on.
 *
 * With `knownOnly` it comes from the pool. Without (or if the pool is empty
 * for these leagues) it is fully random: an enabled club against either a
 * random nation, `NATION_CHANCE` of the time when countries are allowed, or a
 * different enabled club. Such a pair may have no answer, which is the point
 * of the harder mode.
 *
 * Either way a pair from the last `RECENT_WINDOW` is redrawn, up to
 * `MAX_TRIES` times; after that a repeat is better than no pair.
 */
export function drawPair(ix: GameIndex, opts: DrawOptions, rng: Rng = Math.random): Draw | null {
  if (!opts.clubs.length) return null;
  const recent = new Set(opts.recent);
  const fromPool = opts.knownOnly && opts.pool.length > 0;

  let key = 0;
  for (let tries = 0; tries < MAX_TRIES; tries++) {
    key = fromPool ? pick(opts.pool, rng) : randomKey(ix, opts, rng);
    if (!recent.has(key)) break;
  }

  let [left, right] = splitKey(key);
  if (rng() < 0.5) [left, right] = [right, left];
  return { key, left, right };
}

function randomKey(ix: GameIndex, opts: DrawOptions, rng: Rng): number {
  const a = pick(opts.clubs, rng);
  if (opts.allowNations && ix.nations.length && rng() < NATION_CHANCE) {
    return pairKey(a, nationEntity(ix, Math.floor(rng() * ix.nations.length)));
  }
  if (opts.clubs.length < 2) {
    // A single enabled club cannot face another club. A country is the only
    // valid opponent left, even with countries switched off.
    return pairKey(a, nationEntity(ix, Math.floor(rng() * ix.nations.length)));
  }
  let b = a;
  while (b === a) b = pick(opts.clubs, rng);
  return pairKey(a, b);
}

/** Appends a key, keeping only the newest `RECENT_WINDOW`. */
export function pushRecent(recent: readonly number[], key: number): number[] {
  const next = [...recent, key];
  return next.length > RECENT_WINDOW ? next.slice(next.length - RECENT_WINDOW) : next;
}

// --- presentation helpers -----------------------------------------------------

/** Hand-picked badge codes, from the reference. */
// prettier-ignore
const SHORT: Readonly<Record<string, string>> = {
  rma: 'RMA', bar: 'FCB', psg: 'PSG', mun: 'MUN', mci: 'MCI', liv: 'LFC', che: 'CFC', ars: 'AFC',
  tot: 'THFC', juv: 'JUV', mil: 'ACM', int: 'INT', bay: 'FCB', bvb: 'BVB', atm: 'ATM', gal: 'GS',
  fen: 'FB', bjk: 'BJK', ts: 'TS', fcp: 'FCP', ben: 'SLB', spo: 'SCP', ajx: 'AJX', psv: 'PSV',
  fey: 'FEY', nap: 'SSCN', rom: 'ROMA', laz: 'SSL', lyo: 'OL', mar: 'OM', mon: 'ASM', lev: 'B04',
  s04: 'S04', whu: 'WHU', new: 'NUFC', avl: 'AVFC', eve: 'EFC', lee: 'LUFC', wol: 'WOL',
  ibfk: 'İBFK', boc: 'CABJ', riv: 'CARP', mia: 'MIA', hil: 'HIL', nas: 'NSR', cel: 'CEL',
  ran: 'RFC', zen: 'ZEN', spm: 'SPM', csk: 'CSKA', dyn: 'DYN', sha2: 'SHA', and: 'RSCA',
  cbr: 'CLB', oly: 'OLY', pao: 'PAO', aek: 'AEK', paok: 'PAOK', rbs: 'RBS', fcb: 'FCB', yb: 'YB',
  lag: 'LAG', lafc: 'LAFC', nyc: 'NYC', nyrb: 'RBNY',
};

const FILLER = /^(fc|cf|ac|as|sc|cd|ud|rc|us|sl|afc|club|de|la|of|the|1\.|real)$/i;

/**
 * The letters on a club's badge: the hand-picked code, else the initials of
 * the name without filler words (up to three), else the first three letters
 * of a one-word name.
 */
export function shortCode(club: Pick<Club, 'id' | 'name'>): string {
  const fixed = SHORT[club.id];
  if (fixed) return fixed;
  const words = club.name
    .replace(/[.']/g, '')
    .split(/[\s-]+/)
    .filter((w) => w && !FILLER.test(w));
  if (words.length >= 2)
    return words
      .slice(0, 3)
      .map((w) => w[0])
      .join('')
      .toUpperCase();
  return (words[0] ?? club.name).slice(0, 3).toUpperCase();
}

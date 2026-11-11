import { DOCUMENT, isPlatformBrowser } from '@angular/common';
import {
  DestroyRef,
  Injectable,
  PLATFORM_ID,
  computed,
  inject,
  linkedSignal,
  signal,
} from '@angular/core';

import { GAME_READER } from '../../../core/api/games';
import { LevelCode } from '../../../core/models/games';
import { GAME_STRINGS, GameLang, GamesI18n } from '../games-i18n';
import {
  Draw,
  GameIndex,
  Rng,
  TOP5,
  buildIndex,
  buildLevelPool,
  buildPairPool,
  drawFromLevel,
  drawPair,
  enabledClubs,
  findLevel,
  pushRecent,
} from './played-for-both.engine';
import { AnswerView, SideView, answerView, sideView } from './played-for-both.view';

export type { AnswerView, SideView } from './played-for-both.view';
export { SHOWN_ANSWERS } from './played-for-both.view';

export const LEAGUES_STORAGE_KEY = 'scoutpulse.games.played-for-both.leagues';
export const DIFFICULTY_STORAGE_KEY = 'scoutpulse.games.played-for-both.difficulty';

export type TimerSeconds = 0 | 20 | 30 | 60;

/** A level, or the hand-picked leagues and options of the settings panel. */
export type Difficulty = LevelCode | 'custom';

export interface LeagueOption {
  code: string;
  country: string;
  name: string;
  clubs: number;
  on: boolean;
}

/**
 * State for one game of "Who Played For Both?".
 *
 * Provided by the page, so a new visit starts a new game and the timer dies
 * with the page. The rules themselves live in the engine; this is signals,
 * persistence and the timer around them.
 *
 * Expensive work is memoised in `computed`s that depend only on what changes
 * it: the pair pool is rebuilt when the leagues or an option change — and
 * only when a draw next reads it — never per render.
 */
@Injectable()
export class PlayedForBothStore {
  private readonly reader = inject(GAME_READER);
  private readonly i18n = inject(GamesI18n);
  private readonly document = inject(DOCUMENT);
  private readonly isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  /** Replaceable in tests. */
  rng: Rng = Math.random;

  // --- data -------------------------------------------------------------
  readonly loadState = signal<'loading' | 'ready' | 'error'>('loading');
  readonly index = signal<GameIndex | null>(null);

  // --- settings ---------------------------------------------------------
  readonly leagues = signal<ReadonlySet<string>>(new Set());
  readonly knownOnly = signal(true);
  readonly famousOnly = signal(true);
  readonly allowNations = signal(true);
  readonly timerSeconds = signal<TimerSeconds>(30);
  /** As chosen. */
  readonly difficulty = signal<Difficulty>('medium');

  /** The level in force: none for custom, or when the data has no levels. */
  readonly level = computed(() => {
    const ix = this.index();
    const d = this.difficulty();
    return ix && d !== 'custom' ? (findLevel(ix, d) ?? null) : null;
  });

  /** What the picker shows as selected. */
  readonly effectiveDifficulty = computed<Difficulty>(() => this.level()?.code ?? 'custom');

  /** Whether the dataset offers levels at all. */
  readonly hasLevels = computed(() => (this.index()?.levels.length ?? 0) > 0);

  private readonly levelPool = computed(() => {
    const ix = this.index();
    const level = this.level();
    return ix && level ? buildLevelPool(ix, level) : null;
  });

  private readonly clubs = computed(() => {
    const ix = this.index();
    return ix ? enabledClubs(ix, this.leagues()) : [];
  });

  private readonly pool = computed(() => {
    const ix = this.index();
    if (!ix || !this.knownOnly()) return [];
    return buildPairPool(ix, {
      leagues: this.leagues(),
      famousOnly: this.famousOnly(),
      allowNations: this.allowNations(),
    });
  });

  // --- the game ---------------------------------------------------------
  readonly score = signal<readonly [number, number]>([0, 0]);
  readonly round = signal(1);
  readonly pair = signal<Draw | null>(null);
  /** Bumped on every draw, so the emblems re-enter even if a team repeats. */
  readonly drawId = signal(0);
  readonly revealed = signal(false);
  readonly needLeague = signal(false);
  private recent: number[] = [];

  readonly started = computed(() => this.pair() !== null);

  /**
   * The two names as typed. A name still at a default — in either language —
   * or left empty follows a language switch; anything typed stays.
   */
  readonly names = linkedSignal<GameLang, readonly [string, string]>({
    source: this.i18n.lang,
    computation: (lang, previous) => {
      const fresh = (n: 1 | 2) => GAME_STRINGS[lang].player(n);
      if (!previous) return [fresh(1), fresh(2)];
      const keep = (value: string, n: 1 | 2) =>
        value === '' || Object.values(GAME_STRINGS).some((s) => s.player(n) === value)
          ? fresh(n)
          : value;
      return [keep(previous.value[0], 1), keep(previous.value[1], 2)];
    },
  });

  /** What the +1 buttons say: the name, or the default when it is blank. */
  readonly buttonNames = computed(() => {
    const t = this.i18n.t();
    const [a, b] = this.names();
    return [a.trim() || t.player(1), b.trim() || t.player(2)] as const;
  });

  readonly sides = computed<readonly [SideView, SideView] | null>(() => {
    const ix = this.index();
    const p = this.pair();
    if (!ix || !p) return null;
    const t = this.i18n.t();
    const lang = this.i18n.lang();
    return [sideView(ix, p.left, t, lang), sideView(ix, p.right, t, lang)];
  });

  readonly answer = computed<AnswerView | null>(() => {
    const ix = this.index();
    const p = this.pair();
    if (!ix || !p || !this.revealed()) return null;
    return answerView(ix, p.left, p.right, this.i18n.t(), this.i18n.lang());
  });

  readonly leagueOptions = computed<LeagueOption[]>(() => {
    const ix = this.index();
    if (!ix) return [];
    const t = this.i18n.t();
    const on = this.leagues();
    return ix.leagues.map((l, i) => ({
      code: l.code,
      country: t.country(l.country),
      name: l.name,
      clubs: ix.clubsPerLeague[i],
      on: on.has(l.code),
    }));
  });

  readonly counts = computed(() => {
    const ix = this.index();
    return {
      players: ix?.players.length ?? 0,
      clubs: ix?.clubs.length ?? 0,
      nations: ix?.nations.length ?? 0,
      leagues: ix?.leagues.length ?? 0,
    };
  });

  // --- timer ------------------------------------------------------------
  /** 1 → 0 while a round runs. */
  readonly timeLeft = signal(1);
  readonly timerVisible = computed(() => this.timerSeconds() > 0 && this.started());
  private timerId: ReturnType<typeof setInterval> | null = null;
  private endsAt = 0;

  constructor() {
    inject(DestroyRef).onDestroy(() => this.stopTimer());
    // The dataset is browser-only. The server renders the loading state,
    // which is also what the browser's first pass draws, so hydration matches.
    if (this.isBrowser) void this.load();
  }

  // --- loading ----------------------------------------------------------

  async load(): Promise<void> {
    this.loadState.set('loading');
    try {
      const wire = await this.reader.playedForBothDataset();
      const ix = buildIndex(wire);
      this.leagues.set(this.storedLeagues(ix));
      this.difficulty.set(this.storedDifficulty());
      this.index.set(ix);
      this.loadState.set('ready');
    } catch {
      this.loadState.set('error');
    }
  }

  // --- flow ---------------------------------------------------------------

  /** Start game, or Next pair: skips without a point. */
  primary(): void {
    if (this.started()) this.round.update((r) => r + 1);
    this.next();
  }

  reveal(): void {
    if (!this.pair()) return;
    this.revealed.set(true);
    this.stopTimer();
  }

  point(player: 1 | 2): void {
    if (!this.pair()) return;
    this.score.update(([a, b]) => (player === 1 ? [a + 1, b] : [a, b + 1]));
    this.round.update((r) => r + 1);
    this.next();
  }

  reset(): void {
    this.stopTimer();
    this.score.set([0, 0]);
    this.round.set(1);
    this.pair.set(null);
    this.recent = [];
    this.revealed.set(false);
    this.needLeague.set(false);
    this.timeLeft.set(1);
  }

  setName(player: 1 | 2, value: string): void {
    this.names.update(([a, b]) => (player === 1 ? [value, b] : [a, value]));
  }

  private next(): void {
    const ix = this.index();
    if (!ix) return;
    const levelPool = this.levelPool();
    const draw = levelPool
      ? drawFromLevel(levelPool, this.recent, this.rng)
      : drawPair(
          ix,
          {
            clubs: this.clubs(),
            pool: this.pool(),
            knownOnly: this.knownOnly(),
            allowNations: this.allowNations(),
            recent: this.recent,
          },
          this.rng,
        );
    if (!draw) {
      this.needLeague.set(true);
      return;
    }
    this.recent = pushRecent(this.recent, draw.key);
    this.needLeague.set(false);
    this.revealed.set(false);
    this.pair.set(draw);
    this.drawId.update((n) => n + 1);
    this.startTimer();
  }

  // --- settings -------------------------------------------------------------
  // League changes apply from the next pair; nothing here draws.

  toggleLeague(code: string, on: boolean): void {
    const next = new Set(this.leagues());
    if (on) next.add(code);
    else next.delete(code);
    this.setLeagues(next);
  }

  allLeagues(): void {
    this.setLeagues(new Set(this.index()?.leagues.map((l) => l.code) ?? []));
  }

  top5(): void {
    this.setLeagues(new Set(TOP5));
  }

  clearLeagues(): void {
    this.setLeagues(new Set());
  }

  /** Applies from the next pair, like the league settings. */
  setDifficulty(difficulty: Difficulty): void {
    this.difficulty.set(difficulty);
    this.store(DIFFICULTY_STORAGE_KEY, difficulty);
  }

  setTimer(seconds: TimerSeconds): void {
    this.timerSeconds.set(seconds);
    if (this.pair() && !this.revealed()) this.startTimer();
    else this.stopTimer();
  }

  private setLeagues(leagues: ReadonlySet<string>): void {
    this.leagues.set(leagues);
    this.store(LEAGUES_STORAGE_KEY, JSON.stringify([...leagues]));
  }

  private store(key: string, value: string): void {
    try {
      this.document.defaultView?.localStorage.setItem(key, value);
    } catch {
      // Storage blocked: the choice holds for this visit only.
    }
  }

  private storedDifficulty(): Difficulty {
    try {
      const raw = this.document.defaultView?.localStorage.getItem(DIFFICULTY_STORAGE_KEY);
      if (raw === 'easy' || raw === 'medium' || raw === 'hard' || raw === 'custom') return raw;
    } catch {
      // Unreadable or blocked: the default.
    }
    return 'medium';
  }

  private storedLeagues(ix: GameIndex): ReadonlySet<string> {
    const all = ix.leagues.map((l) => l.code);
    try {
      const raw = this.document.defaultView?.localStorage.getItem(LEAGUES_STORAGE_KEY);
      const stored: unknown = raw ? JSON.parse(raw) : null;
      if (Array.isArray(stored)) {
        const valid = stored.filter((c): c is string => typeof c === 'string' && all.includes(c));
        if (valid.length) return new Set(valid);
      }
    } catch {
      // Unreadable or blocked: start with everything on.
    }
    return new Set(all);
  }

  // --- timer ----------------------------------------------------------------

  private startTimer(): void {
    this.stopTimer();
    const seconds = this.timerSeconds();
    this.timeLeft.set(1);
    if (!seconds || !this.isBrowser) return;
    const total = seconds * 1000;
    this.endsAt = Date.now() + total;
    this.timerId = setInterval(() => {
      const left = Math.max(0, this.endsAt - Date.now());
      this.timeLeft.set(left / total);
      if (!left) this.reveal();
    }, 250);
  }

  private stopTimer(): void {
    if (this.timerId !== null) clearInterval(this.timerId);
    this.timerId = null;
  }
}

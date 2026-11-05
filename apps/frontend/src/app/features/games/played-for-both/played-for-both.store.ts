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
import { GAME_STRINGS, GameLang, GamesI18n, clubName, nationName } from '../games-i18n';
import {
  Draw,
  GameIndex,
  Player,
  Rng,
  TOP5,
  buildIndex,
  buildPairPool,
  drawPair,
  enabledClubs,
  isClub,
  matches,
  pushRecent,
  shortCode,
} from './played-for-both.engine';

export const LEAGUES_STORAGE_KEY = 'scoutpulse.games.played-for-both.leagues';

/** How many answer chips to show before "+N more". */
export const SHOWN_ANSWERS = 40;

export type TimerSeconds = 0 | 20 | 30 | 60;

/** One side of the pitch, ready to render. */
export interface SideView {
  kind: 'club' | 'nation';
  label: string;
  /** "Club · England" or "National team". */
  pill: string;
  /** For clubs. */
  code?: string;
  colour1?: string;
  colour2?: string;
  /** For nations: the flag-icons code. */
  flag?: string;
  /** Accessible name of the badge or flag. */
  alt: string;
}

export interface LeagueOption {
  code: string;
  country: string;
  name: string;
  clubs: number;
  on: boolean;
}

export interface AnswerView {
  shown: Player[];
  more: number;
  /** The "no known answer" message, when there are none. */
  none: string | null;
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
    return [this.side(ix, p.left), this.side(ix, p.right)];
  });

  readonly answer = computed<AnswerView | null>(() => {
    const ix = this.index();
    const p = this.pair();
    if (!ix || !p || !this.revealed()) return null;
    const list = matches(ix, p.left, p.right);
    return {
      shown: list.slice(0, SHOWN_ANSWERS),
      more: Math.max(0, list.length - SHOWN_ANSWERS),
      none: list.length
        ? null
        : this.i18n.t().none(this.label(ix, p.left), this.label(ix, p.right)),
    };
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
    const draw = drawPair(
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

  setTimer(seconds: TimerSeconds): void {
    this.timerSeconds.set(seconds);
    if (this.pair() && !this.revealed()) this.startTimer();
    else this.stopTimer();
  }

  private setLeagues(leagues: ReadonlySet<string>): void {
    this.leagues.set(leagues);
    try {
      this.document.defaultView?.localStorage.setItem(
        LEAGUES_STORAGE_KEY,
        JSON.stringify([...leagues]),
      );
    } catch {
      // Storage blocked: the selection holds for this visit only.
    }
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

  // --- views ------------------------------------------------------------------

  private label(ix: GameIndex, e: number): string {
    const lang = this.i18n.lang();
    return isClub(ix, e)
      ? clubName(ix.clubs[e], lang)
      : nationName(ix.nations[e - ix.clubCount], lang);
  }

  private side(ix: GameIndex, e: number): SideView {
    const t = this.i18n.t();
    const label = this.label(ix, e);
    if (isClub(ix, e)) {
      const c = ix.clubs[e];
      return {
        kind: 'club',
        label,
        pill: `${t.club} · ${t.country(ix.leagues[c.league].country)}`,
        code: shortCode(c),
        colour1: c.colour1,
        colour2: c.colour2,
        alt: t.badgeOf(label),
      };
    }
    return {
      kind: 'nation',
      label,
      pill: t.nation,
      flag: ix.nations[e - ix.clubCount].flag,
      alt: t.flagOf(label),
    };
  }
}

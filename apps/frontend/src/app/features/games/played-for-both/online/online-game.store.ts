import { DOCUMENT, isPlatformBrowser } from '@angular/common';
import { DestroyRef, Injectable, PLATFORM_ID, computed, inject, signal } from '@angular/core';

import { ApiError } from '../../../../core/api/api-error';
import { GAME_ROOMS } from '../../../../core/api/games';
import { RoomSeat, RoomSettings, RoomSnapshot } from '../../../../core/models/games';
import { GamesI18n } from '../../games-i18n';
import { GameIndex } from '../played-for-both.engine';
import { AnswerView, SideView, answerView, knownEntity, sideView } from '../played-for-both.view';

export const SEAT_STORAGE_KEY = 'scoutpulse.games.played-for-both.seat';
export const NAME_STORAGE_KEY = 'scoutpulse.games.played-for-both.name';

type StoreKind = 'local' | 'session';

/** Silence on the stream after which the room is polled instead. */
const STREAM_SILENCE_MS = 15_000;
/** How long a new stream may take to send the room. */
const FIRST_EVENT_MS = 4_000;
const POLL_MS = 1_000;
/** Before opening a stream again after the browser gave up on one. */
const RETRY_MS = 2_000;

/** Whether this tab holds a seat, for the page to open on the online game. */
export function hasStoredSeat(doc: Document): boolean {
  try {
    return !!doc.defaultView?.sessionStorage.getItem(SEAT_STORAGE_KEY);
  } catch {
    return false;
  }
}

export type Connection = 'idle' | 'connecting' | 'live' | 'reconnecting';

/** What the last guess came to, shown under the box. */
export type Feedback = { kind: 'wrong' | 'late'; id: number } | null;

/**
 * One player's view of an online room.
 *
 * The server runs the game; this keeps the seat, listens to the room's event
 * stream and sends what the player does. Each event is the whole room, so
 * the state here is simply the newest one received.
 *
 * The seat is kept in sessionStorage: a reload or a trip to another page and
 * back takes the player straight back to their game, while a new tab is a
 * new player.
 */
@Injectable()
export class OnlineGameStore {
  private readonly rooms = inject(GAME_ROOMS);
  private readonly i18n = inject(GamesI18n);
  private readonly document = inject(DOCUMENT);
  private readonly isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  /** Set by the page once the dataset is loaded. */
  readonly index = signal<GameIndex | null>(null);

  readonly seat = signal<RoomSeat | null>(null);
  readonly room = signal<RoomSnapshot | null>(null);
  readonly connection = signal<Connection>('idle');
  readonly busy = signal(false);
  /** A sentence for the player, from the last failed action. */
  readonly error = signal<string | null>(null);
  readonly feedback = signal<Feedback>(null);
  readonly name = signal('');

  private source: EventSource | null = null;
  private feedbackId = 0;

  // --- the fallback ------------------------------------------------------------
  // The stream passes through proxies (Vercel's rewrite, in production) that
  // could hold it back. The server sends something at least every ten
  // seconds, so a longer silence means it is not getting through, and the
  // room is fetched once a second instead until it does.
  private heardAt = 0;
  private watchdog: ReturnType<typeof setInterval> | null = null;
  private polling = false;

  // --- the clock ------------------------------------------------------------
  /** 1 → 0 while a pair is up. */
  readonly timeLeft = signal(1);
  private deadline = 0;
  private duration = 1;
  private tick: ReturnType<typeof setInterval> | null = null;

  // --- derived ----------------------------------------------------------------

  readonly me = computed(() => {
    const id = this.seat()?.playerId;
    return this.room()?.players.find((p) => p.id === id) ?? null;
  });

  readonly isHost = computed(() => {
    const room = this.room();
    return !!room && room.host === this.seat()?.playerId;
  });

  readonly hostName = computed(() => {
    const room = this.room();
    return room?.players.find((p) => p.id === room.host)?.name ?? '';
  });

  /** Players by score, best first; ties keep the order they joined in. */
  readonly standings = computed(() =>
    [...(this.room()?.players ?? [])].sort((a, b) => b.score - a.score),
  );

  readonly onlineCount = computed(() => this.room()?.players.filter((p) => p.online).length ?? 0);

  readonly sides = computed<readonly [SideView, SideView] | null>(() => {
    const ix = this.index();
    const pair = this.room()?.pair;
    if (!ix || !pair || !knownEntity(ix, pair.left) || !knownEntity(ix, pair.right)) return null;
    const t = this.i18n.t();
    const lang = this.i18n.lang();
    return [sideView(ix, pair.left, t, lang), sideView(ix, pair.right, t, lang)];
  });

  /** The round number doubles as the draw id: it changes with every pair. */
  readonly drawId = computed(() => this.room()?.round ?? 0);

  readonly answer = computed<AnswerView | null>(() => {
    const ix = this.index();
    const room = this.room();
    if (!ix || !room?.pair || (room.phase !== 'reveal' && room.phase !== 'finished')) return null;
    if (!knownEntity(ix, room.pair.left) || !knownEntity(ix, room.pair.right)) return null;
    return answerView(ix, room.pair.left, room.pair.right, this.i18n.t(), this.i18n.lang());
  });

  /** "Ali got it: Didier Drogba", or that nobody did. */
  readonly verdict = computed(() => {
    const room = this.room();
    if (!room || (room.phase !== 'reveal' && room.phase !== 'finished')) return null;
    const t = this.i18n.t();
    if (!room.win) return { text: t.nobody, mine: false };
    const mine = room.win.player === this.seat()?.playerId;
    const who = room.players.find((p) => p.id === room.win!.player)?.name ?? '?';
    return { text: mine ? t.youGotIt(room.win.answer) : t.gotIt(who, room.win.answer), mine };
  });

  readonly championName = computed(() => {
    const room = this.room();
    if (!room?.champion) return null;
    if (room.champion === this.seat()?.playerId) return this.i18n.t().youWin;
    const name = room.players.find((p) => p.id === room.champion)?.name ?? '?';
    return this.i18n.t().champion(name);
  });

  readonly shareLink = computed(() => {
    const code = this.seat()?.code;
    const origin = this.document.defaultView?.location.origin ?? '';
    return code ? `${origin}/games/played-for-both?room=${code}` : '';
  });

  constructor() {
    inject(DestroyRef).onDestroy(() => this.disconnect());
    if (!this.isBrowser) return;
    this.name.set(this.read('local', NAME_STORAGE_KEY) ?? '');
    const stored = this.storedSeat();
    if (stored) {
      this.seat.set(stored);
      this.connect(stored);
    }
  }

  // --- joining ----------------------------------------------------------------

  setName(value: string): void {
    this.name.set(value);
    this.write('local', NAME_STORAGE_KEY, value.trim());
  }

  create(): Promise<void> {
    return this.enter(() => this.rooms.create(this.name().trim()));
  }

  join(code: string): Promise<void> {
    const clean = code.trim().toUpperCase();
    if (!clean) return Promise.resolve();
    // Already sitting in that room (a reload of a shared link): stay put.
    if (this.seat()?.code === clean) return Promise.resolve();
    return this.enter(() => this.rooms.join(clean, this.name().trim()));
  }

  private async enter(open: () => Promise<RoomSeat>): Promise<void> {
    if (this.busy()) return;
    this.busy.set(true);
    this.error.set(null);
    try {
      const seat = await open();
      this.leaveQuietly();
      this.seat.set(seat);
      this.write('session', SEAT_STORAGE_KEY, JSON.stringify(seat));
      this.connect(seat);
    } catch (err) {
      this.fail(err);
    } finally {
      this.busy.set(false);
    }
  }

  /** Leaves the room for good. */
  leave(): void {
    this.leaveQuietly();
    this.error.set(null);
  }

  private leaveQuietly(): void {
    const seat = this.seat();
    if (seat) this.rooms.leave(seat);
    this.forget();
  }

  // --- playing ----------------------------------------------------------------

  configure(change: Partial<RoomSettings>): Promise<void> {
    const room = this.room();
    if (!room) return Promise.resolve();
    const settings = { ...room.settings, ...change };
    // Shown at once; the server's next snapshot confirms it.
    this.room.set({ ...room, settings });
    return this.act((seat) => this.rooms.configure(seat, settings));
  }

  start(): Promise<void> {
    return this.act((seat) => this.rooms.start(seat));
  }

  next(): Promise<void> {
    return this.act((seat) => this.rooms.next(seat));
  }

  stop(): Promise<void> {
    return this.act((seat) => this.rooms.stop(seat));
  }

  /** Sends a guess; true when it took the round. */
  async guess(text: string): Promise<boolean> {
    const seat = this.seat();
    if (!seat || !text.trim() || this.room()?.phase !== 'playing') return false;
    try {
      const res = await this.rooms.guess(seat, text);
      if (res.correct) {
        this.feedback.set(null);
        return true;
      }
      this.feedback.set({ kind: res.late ? 'late' : 'wrong', id: ++this.feedbackId });
    } catch (err) {
      // Typing too fast is not worth a message; anything else is.
      if (!(err instanceof ApiError && err.code === 'rate_limited')) this.fail(err);
    }
    return false;
  }

  private async act(call: (seat: RoomSeat) => Promise<void>): Promise<void> {
    const seat = this.seat();
    if (!seat) return;
    this.error.set(null);
    try {
      await call(seat);
    } catch (err) {
      this.fail(err);
    }
  }

  private fail(err: unknown): void {
    const t = this.i18n.t();
    this.error.set(err instanceof ApiError && err.message ? err.message : t.roomError);
  }

  // --- the stream -------------------------------------------------------------

  private connect(seat: RoomSeat): void {
    this.disconnect();
    if (!this.isBrowser || typeof EventSource === 'undefined') return;
    this.connection.set('connecting');
    const source = new EventSource(this.rooms.eventsUrl(seat));
    this.source = source;

    this.heardAt = Date.now() - STREAM_SILENCE_MS + FIRST_EVENT_MS;
    this.startWatchdog(seat);

    source.onopen = () => this.connection.set('live');
    source.addEventListener('ping', () => (this.heardAt = Date.now()));
    source.onmessage = (event: MessageEvent<string>) => {
      this.heardAt = Date.now();
      try {
        this.receive(JSON.parse(event.data) as RoomSnapshot);
      } catch {
        // A garbled event; the next one carries the whole room again.
      }
    };
    // Sent when this player left, or the room was closed.
    source.addEventListener('closed', () => this.ended());
    source.onerror = () => {
      if (this.source !== source) return;
      this.connection.set('reconnecting');
      // CLOSED: the browser gave up on the stream. That happens when the
      // room is gone, but also on a gateway error mid-deploy, or as the page
      // unloads. Only the server can say which, so ask it once: the poll
      // ends the seat on a 404 or 401, and otherwise the stream is retried.
      if (source.readyState === EventSource.CLOSED) {
        void this.pollOnce(seat).then(() => {
          if (this.source === source && this.seat() === seat) {
            setTimeout(() => this.source === source && this.connect(seat), RETRY_MS);
          }
        });
      }
    };
  }

  private receive(room: RoomSnapshot): void {
    this.connection.set('live');
    const before = this.room();
    if (before && before.code === room.code && room.version < before.version) return;
    if (before?.round !== room.round || before.phase !== room.phase) this.feedback.set(null);
    this.room.set(room);
    this.syncClock(room);
  }

  private ended(): void {
    const had = this.room() !== null;
    this.forget();
    if (had) this.error.set(this.i18n.t().roomGone);
  }

  private forget(): void {
    this.disconnect();
    this.seat.set(null);
    this.room.set(null);
    this.feedback.set(null);
    this.remove('session', SEAT_STORAGE_KEY);
  }

  private startWatchdog(seat: RoomSeat): void {
    this.watchdog = setInterval(() => {
      if (Date.now() - this.heardAt > STREAM_SILENCE_MS) void this.pollOnce(seat);
    }, POLL_MS);
  }

  private async pollOnce(seat: RoomSeat): Promise<void> {
    if (this.polling || this.seat() !== seat) return;
    this.polling = true;
    try {
      const room = await this.rooms.poll(seat);
      if (this.seat() === seat) this.receive(room);
    } catch (err) {
      const gone = err instanceof ApiError && (err.status === 404 || err.status === 401);
      if (gone && this.seat() === seat) this.ended();
    } finally {
      this.polling = false;
    }
  }

  private disconnect(): void {
    if (this.watchdog) clearInterval(this.watchdog);
    this.watchdog = null;
    const source = this.source;
    this.source = null;
    source?.close();
    this.connection.set('idle');
    this.stopClock();
  }

  // --- the clock ----------------------------------------------------------------

  private syncClock(room: RoomSnapshot): void {
    if (room.phase !== 'playing' || !room.pair) {
      this.stopClock();
      this.timeLeft.set(room.phase === 'lobby' ? 1 : 0);
      return;
    }
    this.duration = Math.max(1, room.pair.durationMs);
    this.deadline = Date.now() + room.pair.remainingMs;
    this.updateClock();
    if (!this.tick) this.tick = setInterval(() => this.updateClock(), 250);
  }

  private updateClock(): void {
    this.timeLeft.set(Math.max(0, this.deadline - Date.now()) / this.duration);
  }

  private stopClock(): void {
    if (this.tick) clearInterval(this.tick);
    this.tick = null;
  }

  // --- storage ----------------------------------------------------------------

  private storedSeat(): RoomSeat | null {
    try {
      const raw = this.read('session', SEAT_STORAGE_KEY);
      const seat = raw ? (JSON.parse(raw) as Partial<RoomSeat>) : null;
      if (seat?.code && seat.playerId && seat.token) return seat as RoomSeat;
    } catch {
      // Unreadable: start fresh.
    }
    return null;
  }

  /** Even naming a blocked store throws, so it is only reached inside a try. */
  private storage(kind: StoreKind): Storage | undefined {
    const w = this.document.defaultView;
    return kind === 'local' ? w?.localStorage : w?.sessionStorage;
  }

  private read(kind: StoreKind, key: string): string | null {
    try {
      return this.storage(kind)?.getItem(key) ?? null;
    } catch {
      return null;
    }
  }

  private write(kind: StoreKind, key: string, value: string): void {
    try {
      this.storage(kind)?.setItem(key, value);
    } catch {
      // Blocked: the seat lasts as long as the page.
    }
  }

  private remove(kind: StoreKind, key: string): void {
    try {
      this.storage(kind)?.removeItem(key);
    } catch {
      // Blocked: nothing was stored.
    }
  }
}

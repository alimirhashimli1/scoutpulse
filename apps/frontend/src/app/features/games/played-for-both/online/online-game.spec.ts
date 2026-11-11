import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';

import { ApiError } from '../../../../core/api/api-error';
import { GAME_ROOMS, GameRooms } from '../../../../core/api/games';
import { PlayedForBothWire, RoomSnapshot } from '../../../../core/models/games';
import { buildIndex } from '../played-for-both.engine';
import { OnlineGame } from './online-game.component';
import { SEAT_STORAGE_KEY } from './online-game.store';

const WIRE: PlayedForBothWire = {
  leagues: [['eng', 'England', 'Premier League']],
  clubs: [
    ['che', 'Chelsea', '#034694', '#ffffff', 0],
    ['mun', 'Manchester United', '#da291c', '#000000', 0],
  ],
  nations: [['CIV', 'Ivory Coast', 'Fildişi Sahili', 'ci']],
  players: [
    ['Juan Mata', -1, [0, 1], 1],
    ['Didier Drogba', 0, [0], 1],
  ],
};

/** Stands in for the browser's EventSource; the test pushes the events. */
class FakeEventSource {
  static last: FakeEventSource | null = null;
  static readonly CLOSED = 2;
  readyState = 1;
  onopen: (() => void) | null = null;
  onmessage: ((e: MessageEvent<string>) => void) | null = null;
  onerror: (() => void) | null = null;
  private listeners: Record<string, () => void> = {};
  closed = false;

  constructor(readonly url: string) {
    FakeEventSource.last = this;
  }
  addEventListener(type: string, fn: () => void) {
    this.listeners[type] = fn;
  }
  close() {
    this.closed = true;
  }
  push(room: RoomSnapshot) {
    this.onmessage?.({ data: JSON.stringify(room) } as MessageEvent<string>);
  }
  emit(type: string) {
    this.listeners[type]?.();
  }
}

const seat = { code: 'ABCDE', playerId: 'p1', token: 'secret' };

function snapshot(over: Partial<RoomSnapshot> = {}): RoomSnapshot {
  return {
    code: 'ABCDE',
    version: 1,
    phase: 'lobby',
    round: 0,
    host: 'p1',
    players: [
      { id: 'p1', name: 'Ali', score: 0, online: true },
      { id: 'p2', name: 'Bora', score: 0, online: true },
    ],
    settings: { level: 'medium', timer: 30, target: 5 },
    pair: null,
    win: null,
    champion: '',
    ...over,
  };
}

describe('OnlineGame', () => {
  let rooms: { [K in keyof GameRooms]: ReturnType<typeof vi.fn> };

  beforeEach(() => {
    sessionStorage.clear();
    FakeEventSource.last = null;
    vi.stubGlobal('EventSource', FakeEventSource);
    rooms = {
      create: vi.fn().mockResolvedValue(seat),
      join: vi.fn().mockResolvedValue({ ...seat, playerId: 'p2', token: 'other' }),
      configure: vi.fn().mockResolvedValue(undefined),
      start: vi.fn().mockResolvedValue(undefined),
      next: vi.fn().mockResolvedValue(undefined),
      stop: vi.fn().mockResolvedValue(undefined),
      guess: vi.fn().mockResolvedValue({ correct: false }),
      leave: vi.fn(),
      poll: vi.fn(),
      eventsUrl: vi.fn((s: typeof seat) => `/events/${s.code}?token=${s.token}`),
    };
    TestBed.configureTestingModule({
      imports: [OnlineGame],
      providers: [{ provide: GAME_ROOMS, useValue: rooms }],
    });
  });

  afterEach(() => vi.unstubAllGlobals());

  async function setup(code = '') {
    const fixture = TestBed.createComponent(OnlineGame);
    fixture.componentRef.setInput('index', buildIndex(WIRE));
    fixture.componentRef.setInput('initialCode', code);
    await fixture.whenStable();
    const root: HTMLElement = fixture.nativeElement;
    const button = (text: string) =>
      Array.from(root.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.includes(text),
      )!;
    const text = () => root.textContent?.replace(/\s+/g, ' ') ?? '';
    const settle = () => fixture.whenStable();
    const push = async (room: RoomSnapshot) => {
      FakeEventSource.last!.push(room);
      await settle();
    };
    return { fixture, root, button, text, settle, push };
  }

  it('creates a room and listens to it', async () => {
    const { button, root, push, text } = await setup();
    button('Create a game').click();
    await vi.waitFor(() => expect(FakeEventSource.last).not.toBeNull());
    expect(FakeEventSource.last!.url).toBe('/events/ABCDE?token=secret');
    expect(JSON.parse(sessionStorage.getItem(SEAT_STORAGE_KEY)!)).toEqual(seat);

    await push(snapshot({ players: [{ id: 'p1', name: 'Ali', score: 0, online: true }] }));
    expect(root.querySelector('[data-testid="room-code"]')?.textContent).toBe('ABCDE');
    expect(button('Start game').disabled).toBe(true);
    expect(text()).toContain('Waiting for at least one more player');

    await push(snapshot({ version: 2 }));
    expect(button('Start game').disabled).toBe(false);
    button('Start game').click();
    expect(rooms.start).toHaveBeenCalledWith(seat);
  });

  it('joins with the code from a shared link', async () => {
    const { root, button } = await setup('ABCDE');
    const box = root.querySelector<HTMLInputElement>('#pfb-room')!;
    expect(box.value).toBe('ABCDE');
    button('Join').click();
    await vi.waitFor(() => expect(rooms.join).toHaveBeenCalledWith('ABCDE', ''));
  });

  it('comes back to the room after a reload', async () => {
    sessionStorage.setItem(SEAT_STORAGE_KEY, JSON.stringify(seat));
    await setup();
    expect(FakeEventSource.last?.url).toBe('/events/ABCDE?token=secret');
  });

  it('plays a round: a wrong guess, then the reveal', async () => {
    sessionStorage.setItem(SEAT_STORAGE_KEY, JSON.stringify(seat));
    const { root, push, text } = await setup();
    await push(
      snapshot({
        phase: 'playing',
        round: 1,
        pair: { left: 0, right: 1, remainingMs: 15_000, durationMs: 30_000 },
      }),
    );
    expect(text()).toContain('Chelsea');
    expect(text()).toContain('Manchester United');
    expect(root.querySelector('.timer')?.getAttribute('aria-valuenow')).toBe('50');

    const box = root.querySelector<HTMLInputElement>('#pfb-guess')!;
    box.value = 'Lampard';
    root.querySelector('form.guess')!.dispatchEvent(new Event('submit'));
    await vi.waitFor(() => expect(text()).toContain('Not that one'));
    expect(rooms.guess).toHaveBeenCalledWith(seat, 'Lampard');

    await push(
      snapshot({
        version: 3,
        phase: 'reveal',
        round: 1,
        pair: { left: 0, right: 1, remainingMs: 0, durationMs: 30_000 },
        win: { player: 'p2', answer: 'Juan Mata' },
        players: [
          { id: 'p1', name: 'Ali', score: 0, online: true },
          { id: 'p2', name: 'Bora', score: 1, online: true },
        ],
      }),
    );
    expect(text()).toContain('Bora got it: Juan Mata');
    expect(text()).not.toContain('Not that one');
    expect(root.querySelector('app-pfb-answer-list')?.textContent).toContain('Juan Mata');
    // The leader is listed first.
    expect(root.querySelector('.players li')?.textContent).toContain('Bora');
  });

  it('crowns the winner', async () => {
    sessionStorage.setItem(SEAT_STORAGE_KEY, JSON.stringify(seat));
    const { push, text, button } = await setup();
    await push(
      snapshot({
        phase: 'finished',
        round: 5,
        champion: 'p1',
        pair: { left: 0, right: 1, remainingMs: 0, durationMs: 30_000 },
        win: { player: 'p1', answer: 'Juan Mata' },
      }),
    );
    expect(text()).toContain('You win the game!');
    expect(button('Play again')).toBeTruthy();
  });

  it('only the host may change the settings', async () => {
    sessionStorage.setItem(SEAT_STORAGE_KEY, JSON.stringify(seat));
    const { push, root, text } = await setup();
    await push(snapshot({ host: 'p2' }));
    expect(root.querySelector<HTMLFieldSetElement>('app-pfb-difficulty fieldset')!.disabled).toBe(
      true,
    );
    expect(text()).toContain('Waiting for Bora to start the game.');
  });

  it('says so when the room is gone, and forgets the seat', async () => {
    sessionStorage.setItem(SEAT_STORAGE_KEY, JSON.stringify(seat));
    const { push, text, settle, root } = await setup();
    await push(snapshot());
    FakeEventSource.last!.emit('closed');
    await settle();
    expect(text()).toContain('That game has ended.');
    expect(sessionStorage.getItem(SEAT_STORAGE_KEY)).toBeNull();
    expect(root.querySelector('#pfb-room')).not.toBeNull();
  });

  it('shows why joining failed', async () => {
    rooms.join.mockRejectedValue(new ApiError('not_found', 'no game with that code', 404));
    const { button, text, settle } = await setup('ZZZZZ');
    button('Join').click();
    await vi.waitFor(() => expect(rooms.join).toHaveBeenCalled());
    await settle();
    expect(text()).toContain('no game with that code');
  });

  it('polls when the stream does not get through', async () => {
    vi.useFakeTimers();
    try {
      sessionStorage.setItem(SEAT_STORAGE_KEY, JSON.stringify(seat));
      rooms.poll.mockResolvedValue(snapshot({ version: 9 }));
      // Built by hand: whenStable would wait on the fake clock.
      const fixture = TestBed.createComponent(OnlineGame);
      fixture.componentRef.setInput('index', buildIndex(WIRE));
      fixture.detectChanges();
      const root: HTMLElement = fixture.nativeElement;
      vi.advanceTimersByTime(2_000);
      expect(rooms.poll).not.toHaveBeenCalled();
      await vi.advanceTimersByTimeAsync(4_000);
      expect(rooms.poll).toHaveBeenCalledWith(seat);
      fixture.detectChanges();
      expect(root.querySelector('[data-testid="room-code"]')?.textContent).toBe('ABCDE');

      // The stream comes through after all: polling stops.
      FakeEventSource.last!.push(snapshot({ version: 10 }));
      rooms.poll.mockClear();
      await vi.advanceTimersByTimeAsync(5_000);
      expect(rooms.poll).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it('lists a room of thirty as a compact ranked table', async () => {
    sessionStorage.setItem(SEAT_STORAGE_KEY, JSON.stringify(seat));
    const { push, root } = await setup();
    const players = Array.from({ length: 30 }, (_, i) => ({
      id: 'p' + (i + 1),
      name: 'Player ' + (i + 1),
      score: i === 17 ? 4 : 0,
      online: true,
    }));
    await push(snapshot({ players }));
    const list = root.querySelector('.players')!;
    expect(list.classList).toContain('many');
    const rows = list.querySelectorAll('li');
    expect(rows).toHaveLength(30);
    expect(rows[0].textContent).toContain('Player 18');
    expect(rows[0].querySelector('.rank')?.textContent).toBe('1');
  });
});

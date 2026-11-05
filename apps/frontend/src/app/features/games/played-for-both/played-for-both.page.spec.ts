import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { GAME_READER, GameReader } from '../../../core/api/games';
import { PlayedForBothWire } from '../../../core/models/games';
import { SITE_URL } from '../../../core/tokens/site-url';
import { GAME_LANG_STORAGE_KEY } from '../games-i18n';
import { PlayedForBothPage } from './played-for-both.page';

/** Two leagues, four clubs, two nations: enough for every pair to have an answer. */
const WIRE: PlayedForBothWire = {
  leagues: [
    ['eng', 'England', 'Premier League'],
    ['esp', 'Spain', 'LaLiga'],
  ],
  clubs: [
    ['ars', 'Arsenal', '#ef0107', '#ffffff', 0],
    ['tot', 'Tottenham Hotspur', '#132257', '#ffffff', 0],
    ['rma', 'Real Madrid', '#ffffff', '#febe10', 1],
    ['bar', 'Barcelona', '#a50044', '#004d98', 1],
  ],
  nations: [
    ['TOG', 'Togo', 'Togo', 'tg'],
    ['ESP', 'Spain', 'İspanya', 'es'],
  ],
  players: [
    ['Emmanuel Adebayor', 0, [0, 1, 2], 1],
    ['Luis Figo', -1, [2, 3], 1],
    ['Cesc Fàbregas', 1, [0, 3], 1],
  ],
};

describe('PlayedForBothPage', () => {
  beforeEach(() => {
    try {
      localStorage.setItem(GAME_LANG_STORAGE_KEY, 'en');
    } catch {
      /* not needed if storage is unavailable */
    }
    const reader: GameReader = {
      list: () => Promise.resolve([]),
      playedForBothDataset: () => Promise.resolve(WIRE),
    };
    TestBed.configureTestingModule({
      imports: [PlayedForBothPage],
      providers: [
        provideRouter([]),
        { provide: GAME_READER, useValue: reader },
        { provide: SITE_URL, useValue: 'http://test.local' },
      ],
    });
  });

  async function setup() {
    const fixture = TestBed.createComponent(PlayedForBothPage);
    await fixture.whenStable();
    const root: HTMLElement = fixture.nativeElement;
    const buttons = () => Array.from(root.querySelectorAll<HTMLButtonElement>('.controls button'));
    const button = (text: string) => buttons().find((b) => b.textContent?.includes(text))!;
    const text = (sel: string) => root.querySelector(sel)?.textContent?.replace(/\s+/g, ' ').trim();
    const click = async (b: HTMLElement) => {
      b.click();
      await fixture.whenStable();
    };
    return { fixture, root, button, text, click };
  }

  it('runs idle → start → point → reset', async () => {
    const { root, button, text, click } = await setup();

    // Idle: two mystery sides, nothing drawn, only Start enabled.
    expect(root.querySelectorAll('app-team-emblem .q').length).toBe(2);
    expect(text('.pitch')).toContain('Mystery team');
    expect(text('.pitch')).toContain('Press Start for the first pair');
    expect(button('Start game')).toBeTruthy();
    expect(button('Reveal answer').disabled).toBe(true);
    expect(button('+1 Player 1').disabled).toBe(true);
    expect(button('+1 Player 2').disabled).toBe(true);
    expect(text('.score')).toBe('0:0');
    expect(text('.round')).toBe('Round 1');

    // Start draws a pair.
    await click(button('Start game'));
    expect(root.querySelectorAll('app-team-emblem .q').length).toBe(0);
    expect(button('Next pair')).toBeTruthy();
    expect(button('Reveal answer').disabled).toBe(false);
    expect(text('.round')).toBe('Round 1');

    // A point scores, moves the round on and draws the next pair.
    await click(button('+1 Player 1'));
    expect(text('.score')).toBe('1:0');
    expect(text('.round')).toBe('Round 2');

    // Next skips without a point.
    await click(button('Next pair'));
    expect(text('.score')).toBe('1:0');
    expect(text('.round')).toBe('Round 3');

    // Reveal shows who fits.
    await click(button('Reveal answer'));
    expect(root.querySelector('app-pfb-answer-list .chip')).toBeTruthy();

    // Reset returns to idle.
    const reset = Array.from(root.querySelectorAll<HTMLButtonElement>('.settings button')).find(
      (b) => b.textContent?.includes('Reset scores'),
    )!;
    await click(reset);
    expect(root.querySelectorAll('app-team-emblem .q').length).toBe(2);
    expect(button('Start game')).toBeTruthy();
    expect(text('.score')).toBe('0:0');
    expect(text('.round')).toBe('Round 1');
    expect(root.querySelector('app-pfb-answer-list')).toBeNull();
  });

  it('answers the keyboard but not while typing a name', async () => {
    const { fixture, root, text } = await setup();
    const press = async (key: string, target: EventTarget = document.body) => {
      target.dispatchEvent(
        new KeyboardEvent('keydown', { key, code: key === ' ' ? 'Space' : key, bubbles: true }),
      );
      await fixture.whenStable();
    };

    await press(' ');
    expect(root.querySelectorAll('app-team-emblem .q').length).toBe(0);
    await press('2');
    expect(text('.score')).toBe('0:1');

    const name = root.querySelector('app-pfb-scoreboard input')!;
    await press('1', name);
    expect(text('.score')).toBe('0:1');
  });

  it('labels the point buttons with the typed names', async () => {
    const { fixture, root, button } = await setup();
    const input = root.querySelector<HTMLInputElement>('app-pfb-scoreboard input')!;
    input.value = 'Ada';
    input.dispatchEvent(new Event('input'));
    await fixture.whenStable();
    expect(button('+1 Ada')).toBeTruthy();
  });

  it('switches to German, renaming only default player names', async () => {
    const { fixture, root, button, text, click } = await setup();
    const second = root.querySelectorAll<HTMLInputElement>('app-pfb-scoreboard input')[1];
    second.value = 'Ada';
    second.dispatchEvent(new Event('input'));
    await fixture.whenStable();

    const de = Array.from(
      root.querySelectorAll<HTMLButtonElement>('app-games-lang-toggle button'),
    ).find((b) => b.textContent?.trim() === 'DE')!;
    await click(de);

    expect(button('Spiel starten')).toBeTruthy();
    expect(button('+1 Spieler 1')).toBeTruthy();
    expect(button('+1 Ada')).toBeTruthy();
    expect(text('.round')).toBe('Runde 1');
    expect(text('.pitch')).toContain('Unbekanntes Team');
    expect(root.querySelector('.wrap')?.getAttribute('lang')).toBe('de');
  });
});

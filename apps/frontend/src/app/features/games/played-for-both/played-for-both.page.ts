import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';

import { Seo } from '../../../core/seo/seo';
import { ErrorState, Loading } from '../../../shared/ui/states';
import { GamesI18n } from '../games-i18n';
import { AnswerList } from './components/answer-list.component';
import { DifficultyPicker } from './components/difficulty-picker.component';
import { LeaguePicker } from './components/league-picker.component';
import { Pitch } from './components/pitch.component';
import { Scoreboard } from './components/scoreboard.component';
import { OnlineGame } from './online/online-game.component';
import { hasStoredSeat } from './online/online-game.store';
import { PlayedForBothStore, TimerSeconds } from './played-for-both.store';

export type PlayMode = 'local' | 'online';

/**
 * "Who Played For Both?"
 *
 * A pair of teams appears; whoever first names a player who played for both
 * gets the point. Two ways to play: on one screen, with a host pressing the
 * buttons, entirely in the browser; or online, each player on their own
 * device typing names, with the server as referee (see OnlineGame).
 */
@Component({
  selector: 'app-played-for-both',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    RouterLink,
    Loading,
    ErrorState,
    Scoreboard,
    Pitch,
    AnswerList,
    LeaguePicker,
    DifficultyPicker,
    OnlineGame,
  ],
  providers: [PlayedForBothStore],
  host: { '(document:keydown)': 'onKey($event)' },
  template: `
    <div class="page wrap" [attr.lang]="i18n.lang()">
      <div class="crumbs">
        <a routerLink="/games">← {{ t().backToGames }}</a>
      </div>

      <header>
        <h1>{{ t().title }}</h1>
        <p class="sub">{{ t().sub }}</p>
      </header>

      <div class="modes" role="tablist" [attr.aria-label]="t().modes">
        @for (m of modes; track m.value) {
          <button
            type="button"
            role="tab"
            [attr.aria-selected]="mode() === m.value"
            [class.on]="mode() === m.value"
            (click)="setMode(m.value)"
          >
            {{ t()[m.key] }}
          </button>
        }
      </div>

      @switch (store.loadState()) {
        @case ('loading') {
          <app-loading [message]="t().loading" [lines]="5" />
        }
        @case ('error') {
          <app-error-state [message]="t().datasetError" />
          <button type="button" class="btn retry" (click)="store.load()">{{ t().retry }}</button>
        }
        @default {
          @if (mode() === 'online') {
            <app-pfb-online
              [index]="store.index()!"
              [initialCode]="roomCode"
              (seated)="showRoom($event)"
            />
          } @else {
            <app-pfb-scoreboard
              [names]="store.names()"
              [score]="store.score()"
              [round]="store.round()"
              (rename)="store.setName($event.player, $event.value)"
            />

            @if (store.hasLevels()) {
              <app-pfb-difficulty
                [value]="store.effectiveDifficulty()"
                [custom]="true"
                (pick)="store.setDifficulty($event)"
              />
            }

            <app-pfb-pitch [sides]="store.sides()" [drawId]="store.drawId()" />

            @if (store.timerVisible()) {
              <div
                class="timer"
                role="progressbar"
                [attr.aria-label]="t().timeLeft"
                aria-valuemin="0"
                aria-valuemax="100"
                [attr.aria-valuenow]="timerPercent()"
              >
                <i [style.transform]="'scaleX(' + store.timeLeft() + ')'"></i>
              </div>
            }

            <div class="controls">
              <button
                type="button"
                class="btn pt1"
                [disabled]="!store.started()"
                (click)="store.point(1)"
              >
                {{ t().point(store.buttonNames()[0]) }} <kbd>1</kbd>
              </button>
              <button
                type="button"
                class="btn"
                [class.primary]="store.started()"
                [disabled]="!store.started()"
                (click)="store.reveal()"
              >
                {{ t().reveal }} <kbd>R</kbd>
              </button>
              <button
                type="button"
                class="btn pt2"
                [disabled]="!store.started()"
                (click)="store.point(2)"
              >
                {{ t().point(store.buttonNames()[1]) }} <kbd>2</kbd>
              </button>
              <button
                type="button"
                class="btn"
                [class.primary]="!store.started()"
                (click)="store.primary()"
              >
                {{ store.started() ? t().next : t().start }} <kbd>{{ t().space }}</kbd>
              </button>
            </div>

            @if (store.needLeague()) {
              <p class="notice" role="status">{{ t().needLeague }}</p>
            } @else if (store.answer(); as answer) {
              <app-pfb-answer-list [answer]="answer" />
            }

            <details class="settings">
              <summary>{{ t().settings }}</summary>
              <div class="opts">
                @if (store.effectiveDifficulty() !== 'custom') {
                  <p class="note">{{ t().customHint }}</p>
                } @else {
                  <app-pfb-league-picker
                    [leagues]="store.leagueOptions()"
                    (toggle)="store.toggleLeague($event.code, $event.on)"
                    (all)="store.allLeagues()"
                    (top5)="store.top5()"
                    (clear)="store.clearLeagues()"
                  />
                  <label class="opt">
                    <input
                      type="checkbox"
                      [checked]="store.knownOnly()"
                      (change)="store.knownOnly.set(checked($event))"
                    />
                    <span>{{ t().optKnown }}</span>
                  </label>
                  <label class="opt">
                    <input
                      type="checkbox"
                      [checked]="store.famousOnly()"
                      (change)="store.famousOnly.set(checked($event))"
                    />
                    <span>{{ t().optFamous }}</span>
                  </label>
                  <label class="opt">
                    <input
                      type="checkbox"
                      [checked]="store.allowNations()"
                      (change)="store.allowNations.set(checked($event))"
                    />
                    <span>{{ t().optNation }}</span>
                  </label>
                }
                <label class="opt">
                  <span>{{ t().timer }}</span>
                  <select (change)="setTimer($event)">
                    @for (o of timerOptions; track o.value) {
                      <option [value]="o.value" [selected]="store.timerSeconds() === o.value">
                        {{ t()[o.key] }}
                      </option>
                    }
                  </select>
                </label>
                <div>
                  <button type="button" class="btn danger" (click)="store.reset()">
                    {{ t().reset }}
                  </button>
                </div>
                <p class="note">{{ t().note(store.counts()) }}</p>
                <p class="note">{{ t().sources }}</p>
              </div>
            </details>
          }
        }
      }
    </div>
  `,
  styleUrl: './played-for-both.page.css',
})
export class PlayedForBothPage {
  protected readonly store = inject(PlayedForBothStore);
  protected readonly i18n = inject(GamesI18n);
  protected readonly t = this.i18n.t;

  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  /** A room code from a shared link, `?room=ABCDE`. */
  protected readonly roomCode = (this.route.snapshot.queryParamMap.get('room') ?? '')
    .toUpperCase()
    .slice(0, 5);

  /** Online when a link names a room, or this tab is already in one. */
  protected readonly mode = signal<PlayMode>(
    this.roomCode || hasStoredSeat(inject(DOCUMENT)) ? 'online' : 'local',
  );

  protected readonly modes = [
    { value: 'local', key: 'modeLocal' },
    { value: 'online', key: 'modeOnline' },
  ] as const;

  protected readonly timerOptions = [
    { value: 0, key: 'off' },
    { value: 20, key: 's20' },
    { value: 30, key: 's30' },
    { value: 60, key: 's60' },
  ] as const;

  constructor() {
    inject(Seo).describe({
      title: 'Who Played For Both?',
      description:
        'A two-player football trivia game: name a footballer who played for both clubs, or for the club and the country.',
      path: '/games/played-for-both',
    });
  }

  /** Keeps the room in the address, so it can be copied or reloaded. */
  protected showRoom(code: string | null): void {
    const current = this.route.snapshot.queryParamMap.get('room');
    if (code && current !== code) {
      void this.router.navigate([], { queryParams: { room: code }, replaceUrl: true });
    }
  }

  protected setMode(mode: PlayMode): void {
    this.mode.set(mode);
    // Leaving online mode drops a shared code from the address, so a reload
    // does not open it again.
    if (mode === 'local' && this.route.snapshot.queryParamMap.has('room')) {
      void this.router.navigate([], { queryParams: { room: null }, replaceUrl: true });
    }
  }

  protected timerPercent(): number {
    return Math.round(this.store.timeLeft() * 100);
  }

  protected checked(event: Event): boolean {
    return (event.target as HTMLInputElement).checked;
  }

  protected setTimer(event: Event): void {
    this.store.setTimer(Number((event.target as HTMLSelectElement).value) as TimerSeconds);
  }

  /**
   * Space starts or moves on, R reveals, 1 and 2 award the point. Ignored
   * while typing a name or using a form control, and with modifier keys, so
   * browser shortcuts and the settings panel keep working.
   */
  protected onKey(event: KeyboardEvent): void {
    if (this.store.loadState() !== 'ready' || this.mode() !== 'local') return;
    if (event.ctrlKey || event.metaKey || event.altKey || event.repeat) return;
    const target = event.target as HTMLElement | null;
    if (target?.closest?.('input, select, textarea, summary, [contenteditable="true"]')) return;

    if (event.code === 'Space' || event.key === ' ') {
      // Stops the page scrolling, and a focused button from also clicking.
      event.preventDefault();
      this.store.primary();
    } else if (event.key === 'r' || event.key === 'R') {
      this.store.reveal();
    } else if (event.key === '1') {
      this.store.point(1);
    } else if (event.key === '2') {
      this.store.point(2);
    }
  }
}

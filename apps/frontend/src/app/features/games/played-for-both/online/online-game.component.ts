import { NgTemplateOutlet } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';

import { RoomSettings } from '../../../../core/models/games';
import { GamesI18n } from '../../games-i18n';
import { AnswerList } from '../components/answer-list.component';
import { DifficultyPicker } from '../components/difficulty-picker.component';
import { Pitch } from '../components/pitch.component';
import { Difficulty } from '../played-for-both.store';
import { GameIndex } from '../played-for-both.engine';
import { OnlineGameStore } from './online-game.store';

/**
 * The online game: create or join a room, then race the others to type a
 * player who played for both teams. The server decides who was first.
 */
@Component({
  selector: 'app-pfb-online',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [NgTemplateOutlet, Pitch, AnswerList, DifficultyPicker],
  providers: [OnlineGameStore],
  templateUrl: './online-game.component.html',
  styleUrl: './online-game.component.css',
})
export class OnlineGame {
  protected readonly store = inject(OnlineGameStore);
  protected readonly t = inject(GamesI18n).t;

  readonly index = input.required<GameIndex>();
  /** A code from a shared link, to prefill the join box. */
  readonly initialCode = input('');

  /** The room's code once seated, null after leaving. */
  readonly seated = output<string | null>();

  protected readonly timers: RoomSettings['timer'][] = [20, 30, 60];
  protected readonly targets: RoomSettings['target'][] = [3, 5, 10, 0];
  protected readonly copied = signal(false);

  private readonly box = viewChild<ElementRef<HTMLInputElement>>('box');

  constructor() {
    effect(() => this.store.index.set(this.index()));
    effect(() => this.seated.emit(this.store.seat()?.code ?? null));

    // A new pair: empty the box and put the cursor in it.
    effect(() => {
      this.store.drawId();
      const el = this.box()?.nativeElement;
      if (!el) return;
      el.value = '';
      el.focus({ preventScroll: true });
    });

    // A wrong guess: shake the box, again on every miss, and select what was
    // typed so the next try replaces it.
    effect(() => {
      const feedback = this.store.feedback();
      const el = untracked(() => this.box()?.nativeElement);
      if (!el || !feedback) return;
      el.classList.remove('shake');
      void el.offsetWidth;
      if (feedback.kind === 'wrong') el.classList.add('shake');
      el.select();
    });
  }

  protected timerPercent(): number {
    return Math.round(this.store.timeLeft() * 100);
  }

  protected async send(event: Event): Promise<void> {
    event.preventDefault();
    const el = this.box()?.nativeElement;
    if (!el) return;
    await this.store.guess(el.value);
  }

  protected joinRoom(event: Event, code: string): void {
    event.preventDefault();
    void this.store.join(code);
  }

  protected configureLevel(level: Difficulty): void {
    if (level !== 'custom') void this.store.configure({ level });
  }

  protected configureTimer(event: Event): void {
    const timer = Number((event.target as HTMLSelectElement).value) as RoomSettings['timer'];
    void this.store.configure({ timer });
  }

  protected configureTarget(event: Event): void {
    const target = Number((event.target as HTMLSelectElement).value) as RoomSettings['target'];
    void this.store.configure({ target });
  }

  protected async copyLink(): Promise<void> {
    const link = this.store.shareLink();
    try {
      if (navigator.share && matchMedia('(pointer: coarse)').matches) {
        await navigator.share({ url: link, title: this.t().title });
        return;
      }
      await navigator.clipboard.writeText(link);
      this.copied.set(true);
      setTimeout(() => this.copied.set(false), 2000);
    } catch {
      // Sharing cancelled, or no clipboard: the code is on screen anyway.
    }
  }
}

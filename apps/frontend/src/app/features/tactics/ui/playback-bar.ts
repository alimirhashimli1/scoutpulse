import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import { BoardPlayer } from '../board/player';
import { TacticIcon } from './icon';

/**
 * Play, pause, scrub and step through a board. The same bar in the editor and
 * the viewer, so a coach previews exactly what their players will see.
 */
@Component({
  selector: 'app-playback-bar',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TacticIcon],
  template: `
    @let p = player();
    <div class="bar">
      <button
        type="button"
        class="b"
        title="Previous step"
        aria-label="Previous step"
        [disabled]="locked()"
        (click)="p.pause(); p.seekStep(p.step() - (onStepStart() ? 1 : 0))"
      >
        <app-tactic-icon name="prev" />
      </button>
      <button
        type="button"
        class="b play"
        [title]="p.playing() ? 'Pause (space)' : 'Play (space)'"
        [attr.aria-label]="p.playing() ? 'Pause' : 'Play'"
        [disabled]="locked() || !hasMotion()"
        (click)="p.toggle()"
      >
        <app-tactic-icon [name]="p.playing() ? 'pause' : 'play'" />
      </button>
      <button
        type="button"
        class="b"
        title="Next step"
        aria-label="Next step"
        [disabled]="locked()"
        (click)="p.pause(); p.seekStep(p.step() + 1)"
      >
        <app-tactic-icon name="next" />
      </button>
      <button
        type="button"
        class="b"
        title="Play from the start"
        aria-label="Play from the start"
        [disabled]="locked() || !hasMotion()"
        (click)="p.restart()"
      >
        <app-tactic-icon name="restart" />
      </button>

      <div class="track">
        <input
          type="range"
          min="0"
          [max]="p.timeline().total"
          step="0.01"
          [value]="p.time()"
          [disabled]="locked() || !hasMotion()"
          aria-label="Position in the animation"
          (input)="p.pause(); p.seek(+$any($event.target).value)"
        />
        <div class="ticks" aria-hidden="true">
          @for (s of p.timeline().stepStarts; track $index) {
            <span [style.left.%]="pct(s)" [class.past]="p.time() >= s"></span>
          }
        </div>
      </div>

      <span class="clock">
        Step {{ p.step() + 1 }}/{{ p.timeline().stepStarts.length }}
        <span class="muted">· {{ p.time().toFixed(1) }}s</span>
      </span>

      <select
        class="speed"
        aria-label="Speed"
        [value]="p.speed()"
        (change)="p.speed.set(+$any($event.target).value)"
      >
        <option value="0.5">0.5×</option>
        <option value="0.75">0.75×</option>
        <option value="1">1×</option>
        <option value="1.5">1.5×</option>
        <option value="2">2×</option>
      </select>
      <label class="loop" title="Repeat">
        <input
          type="checkbox"
          [checked]="p.loop()"
          (change)="p.loop.set($any($event.target).checked)"
        />
        Loop
      </label>
    </div>
  `,
  styles: `
    .bar {
      display: flex;
      align-items: center;
      gap: var(--space-2);
      flex-wrap: wrap;
      font-size: var(--text-sm);
    }
    .b {
      width: 2.2rem;
      height: 2.2rem;
      display: inline-grid;
      place-items: center;
      border-radius: 50%;
      border: 1px solid var(--line);
      background: var(--surface);
      color: var(--ink-soft);
      cursor: pointer;
    }
    .b:hover:not(:disabled) {
      color: var(--accent);
      border-color: var(--accent);
    }
    .b:disabled {
      opacity: 0.45;
      cursor: default;
    }
    .b.play {
      width: 2.7rem;
      height: 2.7rem;
      background: var(--accent);
      border-color: var(--accent);
      color: var(--accent-ink);
    }
    /* The generic hover tints the icon in the accent, which on this button is
       also the fill — the icon would vanish. Brighten the fill instead. */
    .b.play:hover:not(:disabled) {
      color: var(--accent-ink);
      filter: brightness(1.15);
    }
    .track {
      position: relative;
      flex: 1 1 10rem;
      min-width: 8rem;
      height: 1.6rem;
      display: flex;
      align-items: center;
    }
    .track input {
      width: 100%;
      accent-color: var(--accent);
      position: relative;
      z-index: 1;
    }
    .ticks {
      position: absolute;
      inset: 0 7px;
      pointer-events: none;
    }
    .ticks span {
      position: absolute;
      top: 0;
      width: 2px;
      height: 6px;
      background: var(--line);
      transform: translateX(-1px);
    }
    .ticks span.past {
      background: var(--accent);
    }
    .clock {
      font-variant-numeric: tabular-nums;
      white-space: nowrap;
    }
    .muted {
      color: var(--muted);
    }
    .speed {
      padding: 4px;
      border: 1px solid var(--line);
      border-radius: var(--radius);
      background: var(--surface);
    }
    .loop {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      color: var(--ink-soft);
      accent-color: var(--accent);
    }
  `,
})
export class PlaybackBar {
  readonly player = input.required<BoardPlayer>();
  /** While a video is recording, nothing may move the clock. */
  readonly locked = input(false);

  protected readonly hasMotion = computed(() => this.player().timeline().stepStarts.length > 1);

  /** Whether the clock sits exactly on a step, so "previous" goes back one. */
  protected readonly onStepStart = computed(() => {
    const p = this.player();
    const start = p.timeline().stepStarts[p.step()] ?? 0;
    return Math.abs(p.time() - start) < 0.05;
  });

  protected pct(t: number): number {
    const total = this.player().timeline().total;
    return total ? (t / total) * 100 : 0;
  }
}

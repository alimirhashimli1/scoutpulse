import { ChangeDetectionStrategy, Component, inject, input } from '@angular/core';

import { GamesI18n } from '../../games-i18n';
import { SideView } from '../played-for-both.view';
import { TeamEmblem } from './team-emblem.component';

/**
 * The two teams of a pair, or two "?" before the first one. `drawId` changes
 * on every draw, so the emblems flip in again even when a team repeats.
 */
@Component({
  selector: 'app-pfb-pitch',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TeamEmblem],
  template: `
    <section class="pitch" aria-live="polite">
      @for (side of sidesOrIdle(); track drawId() + '-' + $index) {
        <div class="side" [class.flip]="side">
          <app-team-emblem [side]="side" />
          <div class="name">{{ side?.label ?? t().mystery }}</div>
          <div class="pill">{{ side?.pill ?? idle() ?? t().idle }}</div>
        </div>
        @if ($first) {
          <div class="vs" aria-hidden="true">VS</div>
        }
      }
    </section>
  `,
  host: { '[class.compact]': 'compact()' },
  styleUrl: './pitch.component.css',
})
export class Pitch {
  protected readonly t = inject(GamesI18n).t;

  readonly sides = input<readonly [SideView, SideView] | null>(null);
  readonly drawId = input(0);
  /** What the "?" sides say; defaults to "press Start". */
  readonly idle = input<string | null>(null);
  /**
   * Keeps the two sides next to each other on a phone, smaller, so what
   * comes below (the answer box, online) stays on screen.
   */
  readonly compact = input(false);

  protected sidesOrIdle() {
    return this.sides() ?? [null, null];
  }
}

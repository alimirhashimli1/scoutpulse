import { ChangeDetectionStrategy, Component, inject, input, output } from '@angular/core';

import { GamesI18n } from '../../games-i18n';

/** Two editable names, the score and the round. */
@Component({
  selector: 'app-pfb-scoreboard',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="board" role="group" [attr.aria-label]="i18n.t().scoreboard">
      <div class="team p1">
        <span class="dot" aria-hidden="true"></span>
        <input
          [value]="names()[0]"
          [attr.aria-label]="i18n.t().playerName(1)"
          maxlength="24"
          (input)="rename.emit({ player: 1, value: $any($event.target).value })"
        />
      </div>
      <div class="score tabular" aria-live="polite" aria-atomic="true">
        <span>{{ score()[0] }}</span
        >:<span>{{ score()[1] }}</span>
      </div>
      <div class="team p2">
        <span class="dot" aria-hidden="true"></span>
        <input
          [value]="names()[1]"
          [attr.aria-label]="i18n.t().playerName(2)"
          maxlength="24"
          (input)="rename.emit({ player: 2, value: $any($event.target).value })"
        />
      </div>
      <div class="round">{{ i18n.t().round(round()) }}</div>
    </div>
  `,
  styles: `
    .board {
      background: var(--surface);
      border: 1px solid var(--line);
      border-radius: var(--radius-lg);
      padding: var(--space-3) var(--space-4);
      display: grid;
      grid-template-columns: 1fr auto 1fr;
      align-items: center;
      gap: var(--space-3);
      box-shadow: var(--shadow);
    }
    .team {
      display: flex;
      align-items: center;
      gap: var(--space-3);
      min-width: 0;
    }
    .p2 {
      flex-direction: row-reverse;
    }
    input {
      background: transparent;
      border: 0;
      border-bottom: 1px dashed transparent;
      font: 700 var(--text-lg) / 1.2 var(--font-body);
      text-transform: uppercase;
      letter-spacing: 0.04em;
      width: 100%;
      min-width: 0;
      padding: 2px 0;
    }
    .p2 input {
      text-align: right;
    }
    input:hover,
    input:focus {
      border-bottom-color: var(--muted);
    }
    .dot {
      width: 12px;
      height: 12px;
      border-radius: 50%;
      flex: none;
    }
    .p1 .dot {
      background: var(--pfb-p1);
    }
    .p2 .dot {
      background: var(--pfb-p2);
    }
    .score {
      font: 600 2.6rem / 1 var(--font-mono);
      color: var(--accent);
      letter-spacing: 0.06em;
      white-space: nowrap;
    }
    .score span {
      display: inline-block;
      min-width: 1.3ch;
      text-align: center;
    }
    .round {
      grid-column: 1 / -1;
      text-align: center;
      font-size: var(--text-xs);
      font-weight: 600;
      letter-spacing: 0.18em;
      text-transform: uppercase;
      color: var(--muted);
    }
    @media (max-width: 560px) {
      .score {
        font-size: 2rem;
      }
      input {
        font-size: var(--text-base);
      }
    }
  `,
})
export class Scoreboard {
  protected readonly i18n = inject(GamesI18n);

  readonly names = input.required<readonly [string, string]>();
  readonly score = input.required<readonly [number, number]>();
  readonly round = input.required<number>();

  readonly rename = output<{ player: 1 | 2; value: string }>();
}

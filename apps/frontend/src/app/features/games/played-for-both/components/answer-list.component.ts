import { ChangeDetectionStrategy, Component, inject, input } from '@angular/core';

import { GamesI18n } from '../../games-i18n';
import { AnswerView } from '../played-for-both.store';

/** The revealed answer: up to 40 names as chips, lesser-known ones dimmed. */
@Component({
  selector: 'app-pfb-answer-list',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <h2>{{ i18n.t().fit }}</h2>
    @if (answer().none; as none) {
      <p class="none">{{ none }}</p>
    } @else {
      <ul class="players">
        @for (p of answer().shown; track p) {
          <li class="chip" [class.minor]="!p.famous">{{ p.name }}</li>
        }
        @if (answer().more) {
          <li class="chip more">{{ i18n.t().more(answer().more) }}</li>
        }
      </ul>
    }
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      gap: var(--space-3);
      background: var(--surface);
      border: 1px solid var(--line);
      border-radius: var(--radius-lg);
      padding: var(--space-4);
    }
    h2 {
      font: 700 var(--text-xs) / 1 var(--font-body);
      letter-spacing: 0.2em;
      text-transform: uppercase;
      color: var(--accent);
    }
    .players {
      display: flex;
      flex-wrap: wrap;
      gap: var(--space-2);
      list-style: none;
      margin: 0;
      padding: 0;
    }
    .chip {
      background: var(--surface-2);
      border: 1px solid var(--line);
      border-radius: 99px;
      padding: 6px 12px;
      font-weight: 600;
    }
    .chip.minor {
      font-weight: 400;
      color: var(--muted);
    }
    .chip.more {
      border-style: dashed;
      background: none;
      color: var(--muted);
    }
    .none {
      color: var(--ink-soft);
    }
  `,
})
export class AnswerList {
  protected readonly i18n = inject(GamesI18n);
  readonly answer = input.required<AnswerView>();
}

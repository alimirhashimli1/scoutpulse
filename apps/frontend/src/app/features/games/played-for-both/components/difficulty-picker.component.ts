import { ChangeDetectionStrategy, Component, computed, inject, input, output } from '@angular/core';

import { GamesI18n } from '../../games-i18n';
import { Difficulty } from '../played-for-both.store';

let nextId = 0;

/**
 * Easy, Medium, Hard (and, on one screen, Custom) as one row of radio
 * buttons, with a line saying what the chosen one means.
 */
@Component({
  selector: 'app-pfb-difficulty',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <fieldset [disabled]="disabled()">
      <legend>{{ t().difficulty }}</legend>
      <div class="row">
        @for (o of options(); track o) {
          <label [class.on]="value() === o">
            <input
              type="radio"
              [name]="name"
              [value]="o"
              [checked]="value() === o"
              (change)="pick.emit(o)"
            />
            {{ text()[o].label }}
          </label>
        }
      </div>
      <p class="hint">{{ text()[value()].hint }}</p>
    </fieldset>
  `,
  styles: `
    fieldset {
      border: 0;
      margin: 0;
      padding: 0;
      min-width: 0;
      display: grid;
      gap: var(--space-2);
    }
    legend {
      padding: 0;
      margin-bottom: var(--space-2);
      font-size: var(--text-xs);
      font-weight: 700;
      letter-spacing: 0.18em;
      text-transform: uppercase;
      color: var(--ink-soft);
    }
    .row {
      display: grid;
      grid-auto-flow: column;
      grid-auto-columns: 1fr;
      border: 1px solid var(--line);
      border-radius: var(--radius-lg);
      overflow: hidden;
      background: var(--surface);
    }
    label {
      position: relative;
      display: grid;
      place-items: center;
      min-height: 44px;
      padding: var(--space-2);
      font-weight: 600;
      cursor: pointer;
      color: var(--ink-soft);
    }
    label + label {
      border-left: 1px solid var(--line);
    }
    label.on {
      background: var(--accent);
      color: var(--accent-ink);
    }
    label:has(input:focus-visible) {
      outline: 2px solid var(--accent);
      outline-offset: -4px;
    }
    fieldset:disabled label {
      cursor: default;
    }
    fieldset:disabled label:not(.on) {
      opacity: 0.6;
    }
    input {
      position: absolute;
      opacity: 0;
      pointer-events: none;
    }
    .hint {
      margin: 0;
      font-size: var(--text-sm);
      color: var(--muted);
      max-width: none;
    }
  `,
})
export class DifficultyPicker {
  protected readonly t = inject(GamesI18n).t;
  protected readonly name = `pfb-difficulty-${nextId++}`;

  readonly value = input.required<Difficulty>();
  /** Offer Custom too. */
  readonly custom = input(false);
  readonly disabled = input(false);

  readonly pick = output<Difficulty>();

  protected readonly text = computed(() => {
    const t = this.t();
    return {
      easy: { label: t.easy, hint: t.easyHint },
      medium: { label: t.medium, hint: t.mediumHint },
      hard: { label: t.hard, hint: t.hardHint },
      custom: { label: t.custom, hint: t.customHint },
    } satisfies Record<Difficulty, { label: string; hint: string }>;
  });

  protected readonly options = computed<Difficulty[]>(() =>
    this.custom() ? ['easy', 'medium', 'hard', 'custom'] : ['easy', 'medium', 'hard'],
  );
}

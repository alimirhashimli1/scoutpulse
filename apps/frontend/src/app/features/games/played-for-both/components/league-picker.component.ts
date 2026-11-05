import { ChangeDetectionStrategy, Component, inject, input, output } from '@angular/core';

import { GamesI18n } from '../../games-i18n';
import { LeagueOption } from '../played-for-both.store';

/** One checkbox per league, plus the All / Top 5 / Clear presets. */
@Component({
  selector: 'app-pfb-league-picker',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <fieldset>
      <div class="head">
        <legend>{{ i18n.t().leaguesLbl }}</legend>
        <span class="presets">
          <button type="button" class="btn" (click)="all.emit()">{{ i18n.t().allL }}</button>
          <button type="button" class="btn" (click)="top5.emit()">{{ i18n.t().top5 }}</button>
          <button type="button" class="btn" (click)="clear.emit()">{{ i18n.t().noneL }}</button>
        </span>
      </div>
      <div class="list">
        @for (l of leagues(); track l.code) {
          <label class="league">
            <input
              type="checkbox"
              [checked]="l.on"
              (change)="toggle.emit({ code: l.code, on: $any($event.target).checked })"
            />
            <span>
              {{ l.country }}
              <small>{{ l.name }} · {{ l.clubs }}</small>
            </span>
          </label>
        }
      </div>
    </fieldset>
  `,
  styles: `
    fieldset {
      border: 0;
      margin: 0;
      padding: 0;
      display: grid;
      gap: var(--space-3);
      min-width: 0;
    }
    .head {
      display: flex;
      flex-wrap: wrap;
      gap: var(--space-3);
      align-items: center;
      justify-content: space-between;
    }
    legend {
      float: left;
      padding: 0;
      color: var(--ink);
      font-weight: 600;
    }
    .presets {
      display: flex;
      flex-wrap: wrap;
      gap: var(--space-2);
    }
    .presets .btn {
      padding: var(--space-1) var(--space-3);
      font-size: var(--text-sm);
    }
    .list {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
      gap: var(--space-2);
    }
    .league {
      display: flex;
      gap: var(--space-2);
      align-items: flex-start;
      border: 1px solid var(--line);
      border-radius: var(--radius-lg);
      padding: var(--space-2) var(--space-3);
      cursor: pointer;
      min-width: 0;
      background: var(--surface);
    }
    .league:has(input:checked) {
      border-color: var(--accent);
    }
    .league input {
      margin-top: 4px;
      accent-color: var(--accent);
    }
    .league span {
      display: flex;
      flex-direction: column;
      font-weight: 600;
      color: var(--ink);
      min-width: 0;
    }
    small {
      font-weight: 400;
      color: var(--muted);
      font-size: var(--text-xs);
    }
  `,
})
export class LeaguePicker {
  protected readonly i18n = inject(GamesI18n);

  readonly leagues = input.required<LeagueOption[]>();

  readonly toggle = output<{ code: string; on: boolean }>();
  readonly all = output<void>();
  readonly top5 = output<void>();
  readonly clear = output<void>();
}

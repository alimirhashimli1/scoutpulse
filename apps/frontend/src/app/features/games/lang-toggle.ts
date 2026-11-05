import { ChangeDetectionStrategy, Component, inject } from '@angular/core';

import { GameLang, GamesI18n } from './games-i18n';

/**
 * EN / TR switch for the games section. Exists only because the platform has
 * no i18n of its own; see games-i18n.ts.
 */
@Component({
  selector: 'app-games-lang-toggle',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="lang" role="group" [attr.aria-label]="i18n.t().language">
      @for (l of langs; track l.code) {
        <button
          type="button"
          [attr.lang]="l.code"
          [attr.aria-pressed]="i18n.lang() === l.code"
          (click)="i18n.set(l.code)"
        >
          {{ l.label }}
        </button>
      }
    </div>
  `,
  styles: `
    .lang {
      display: inline-flex;
      border: 1px solid var(--line);
      border-radius: 99px;
      padding: 3px;
      gap: 3px;
      background: var(--surface);
    }
    button {
      border: 0;
      border-radius: 99px;
      padding: 5px 12px;
      font: 700 var(--text-xs) / 1 var(--font-body);
      letter-spacing: 0.12em;
      background: transparent;
      color: var(--muted);
      cursor: pointer;
    }
    button[aria-pressed='true'] {
      background: var(--accent);
      color: var(--accent-ink);
    }
  `,
})
export class GamesLangToggle {
  protected readonly i18n = inject(GamesI18n);
  protected readonly langs: { code: GameLang; label: string }[] = [
    { code: 'en', label: 'EN' },
    { code: 'tr', label: 'TR' },
    { code: 'de', label: 'DE' },
  ];
}

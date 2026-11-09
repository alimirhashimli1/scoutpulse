import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { RouterLink } from '@angular/router';

import { GAME_READER } from '../../../core/api/games';
import { ssrResource } from '../../../core/api/ssr-resource';
import { Seo } from '../../../core/seo/seo';
import { ErrorState, Loading } from '../../../shared/ui/states';
import { GamesI18n } from '../games-i18n';

/**
 * The games section: a card per game, from the server's catalog so a new
 * game does not mean editing this page.
 */
@Component({
  selector: 'app-games-hub',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, Loading, ErrorState],
  template: `
    <section class="page hub" [attr.lang]="i18n.lang()">
      <header class="head">
        <div>
          <h1>{{ t().gamesTitle }}</h1>
          <p class="lede">{{ t().gamesLede }}</p>
        </div>
      </header>

      @if (games.value(); as list) {
        <ul class="grid">
          @for (g of list; track g.slug) {
            <li class="card">
              <h2>{{ i18n.catalog(g.titleKey) }}</h2>
              <p>{{ i18n.catalog(g.descriptionKey) }}</p>
              <div class="foot">
                <span class="tag">{{ t().playersTag(g.players) }}</span>
                @if (g.status === 'live') {
                  <a class="btn primary" [routerLink]="['/games', g.slug]">{{ t().play }}</a>
                } @else {
                  <span class="tag">{{ t().soon }}</span>
                }
              </div>
            </li>
          }
        </ul>
      } @else if (games.error()) {
        <app-error-state [message]="t().hubError" />
        <button type="button" class="btn retry" (click)="games.reload()">{{ t().retry }}</button>
      } @else {
        <app-loading [lines]="3" />
      }
    </section>
  `,
  styles: `
    .hub {
      display: flex;
      flex-direction: column;
      gap: var(--space-5);
      padding-block: var(--space-6);
    }
    .head {
      display: flex;
      flex-wrap: wrap;
      justify-content: space-between;
      align-items: flex-start;
      gap: var(--space-4);
    }
    .lede {
      margin-top: var(--space-2);
      color: var(--ink-soft);
    }
    .grid {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(min(100%, 280px), 1fr));
      gap: var(--space-4);
    }
    .card {
      display: flex;
      flex-direction: column;
      gap: var(--space-3);
      padding: var(--space-5);
      background: var(--surface);
      border: 1px solid var(--line);
      border-radius: var(--radius-lg);
      box-shadow: var(--shadow-sm);
    }
    .card p {
      color: var(--ink-soft);
      flex: 1;
    }
    .foot {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      justify-content: space-between;
      gap: var(--space-3);
    }
    .tag {
      font-family: var(--font-mono);
      font-size: var(--text-xs);
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--accent);
      background: var(--accent-soft);
      padding: 2px 8px;
      border-radius: var(--radius-sm);
    }
    .retry {
      align-self: flex-start;
    }
  `,
})
export class GamesHub {
  protected readonly i18n = inject(GamesI18n);
  protected readonly t = this.i18n.t;
  private readonly reader = inject(GAME_READER);

  protected readonly games = ssrResource('games', {
    loader: () => this.reader.list(),
  });

  constructor() {
    inject(Seo).describe({
      title: 'Games',
      description:
        'Football trivia games to play with a friend, starting with Who Played For Both?',
      path: '/games',
    });
  }
}

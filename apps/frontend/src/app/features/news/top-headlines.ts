import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { RouterLink } from '@angular/router';

import { NEWS_READER } from '../../core/api/news';
import { ssrResource } from '../../core/api/ssr-resource';
import { NewsCard } from './news-card';

/**
 * The home page's "Top Headlines": the best few stories of the last day.
 *
 * Renders nothing at all until there is something to show -- no heading, no
 * skeleton, no error. It is a bonus on a page whose subject is transfers, and
 * an empty or broken box above the transfer table would only push it down.
 */
@Component({
  selector: 'app-top-headlines',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, NewsCard],
  template: `
    @if (stories.value()?.length) {
      <section class="top" aria-labelledby="top-headlines-title">
        <header>
          <h2 id="top-headlines-title">Top Headlines</h2>
          <a routerLink="/news">See all news <span aria-hidden="true">→</span></a>
        </header>
        <ul>
          @for (s of stories.value(); track s.id) {
            <li><app-news-card [story]="s" [compact]="true" /></li>
          }
        </ul>
      </section>
    }
  `,
  styles: `
    .top {
      margin-bottom: var(--space-6);
      padding-bottom: var(--space-6);
      border-bottom: 1px solid var(--line);
    }
    header {
      display: flex;
      align-items: baseline;
      justify-content: space-between;
      gap: var(--space-3);
      margin-bottom: var(--space-4);
    }
    h2 {
      font-size: var(--text-xl);
    }
    header a {
      font-size: var(--text-sm);
      font-weight: 600;
      color: var(--accent);
    }
    ul {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
      gap: var(--space-3);
    }
  `,
})
export class TopHeadlines {
  private readonly reader = inject(NEWS_READER);

  protected readonly stories = ssrResource('top-headlines', {
    // A failure resolves to nothing, which hides the section.
    loader: () => this.reader.top(5).catch(() => []),
  });
}

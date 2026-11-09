import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  signal,
} from '@angular/core';
import { RouterLink } from '@angular/router';

import { NEWS_READER } from '../../core/api/news';
import { ssrResource } from '../../core/api/ssr-resource';
import { NewsStory } from '../../core/models/news';
import { Seo } from '../../core/seo/seo';
import { messageFor } from '../../shared/forms/submit';
import { Empty, ErrorState, Loading } from '../../shared/ui/states';
import { NewsCard } from './news-card';
import { NEWS_CATEGORIES, categoryLabel, isCategory } from './news-meta';

const PAGE = 20;

/**
 * The news section: published stories, pinned first, then newest. `?category=`
 * narrows it, as a link rather than local state, so a filtered view can be
 * shared and the back button undoes a filter.
 */
@Component({
  selector: 'app-news-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, NewsCard, Loading, Empty, ErrorState],
  template: `
    <main class="page section">
      <header class="head">
        <p class="eyebrow">ScoutPulse</p>
        <h1>Football News</h1>
        <p class="lede">
          The day's most important stories for scouts — transfers, injuries, managers and young
          talent — picked from trusted outlets. Read each one at its source.
        </p>
      </header>

      <nav class="filters" aria-label="Filter by category">
        <a
          routerLink="/news"
          [class.active]="!active()"
          [attr.aria-current]="!active() ? 'page' : null"
          >All</a
        >
        @for (c of categories; track c.value) {
          <a
            routerLink="/news"
            [queryParams]="{ category: c.value }"
            [class.active]="active() === c.value"
            [attr.aria-current]="active() === c.value ? 'page' : null"
            >{{ c.label }}</a
          >
        }
      </nav>

      @if (first.hasValue()) {
        @if (stories().length) {
          <ul class="list">
            @for (s of stories(); track s.id) {
              <li><app-news-card [story]="s" /></li>
            }
          </ul>
          @if (hasMore()) {
            <button type="button" class="btn more" [disabled]="loadingMore()" (click)="loadMore()">
              {{ loadingMore() ? 'Loading…' : 'Load more' }}
            </button>
          }
        } @else {
          <app-empty
            [message]="active() ? 'No ' + activeLabel() + ' stories right now.' : 'No news yet.'"
            hint="Stories appear here as they break. Check back soon."
          />
        }
      } @else if (first.isLoading()) {
        <app-loading [lines]="6" message="Loading news…" />
      } @else if (first.error()) {
        <app-error-state message="Could not load the news. Please try again shortly." />
      }
      @if (moreError()) {
        <app-error-state [message]="moreError()!" />
      }
    </main>
  `,
  styles: `
    .section {
      padding-block: var(--space-6) var(--space-8);
      display: flex;
      flex-direction: column;
      gap: var(--space-5);
    }
    .head {
      padding-bottom: var(--space-5);
      border-bottom: 2px solid var(--ink);
    }
    .eyebrow {
      font-size: var(--text-xs);
      font-weight: 700;
      letter-spacing: 0.14em;
      text-transform: uppercase;
      color: var(--accent);
    }
    h1 {
      font-size: var(--text-4xl);
    }
    .lede {
      margin-top: var(--space-2);
      color: var(--ink-soft);
      max-width: var(--measure);
    }
    .filters {
      display: flex;
      flex-wrap: wrap;
      gap: var(--space-2);
    }
    .filters a {
      background: var(--surface);
      border: 1px solid var(--line);
      border-radius: 999px;
      padding: var(--space-1) var(--space-3);
      font-size: var(--text-sm);
      color: var(--ink-soft);
      text-decoration: none;
    }
    .filters a.active {
      border-color: var(--accent);
      color: var(--accent);
      background: var(--accent-soft);
      font-weight: 600;
    }
    .filters a:focus-visible {
      outline: 2px solid var(--accent);
      outline-offset: 2px;
    }
    .list {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(20rem, 1fr));
      gap: var(--space-4);
    }
    @media (max-width: 40rem) {
      h1 {
        font-size: var(--text-3xl);
      }
      .list {
        grid-template-columns: 1fr;
      }
    }
    .more {
      align-self: center;
    }
  `,
})
export class NewsPage {
  /** From the query string, via withComponentInputBinding. */
  readonly category = input<string | undefined>();

  private readonly reader = inject(NEWS_READER);
  protected readonly categories = NEWS_CATEGORIES;
  protected readonly active = computed(() => {
    const c = this.category();
    return isCategory(c) ? c : undefined;
  });
  protected readonly activeLabel = computed(() => {
    const c = this.active();
    return c ? categoryLabel(c) : '';
  });

  protected readonly first = ssrResource('news', {
    params: () => ({ category: this.active() }),
    loader: ({ params }) => this.reader.list({ ...params, limit: PAGE }),
  });

  private readonly extra = signal<NewsStory[]>([]);
  private readonly extraHasMore = signal<boolean | null>(null);
  protected readonly loadingMore = signal(false);
  protected readonly moreError = signal<string | null>(null);

  protected readonly stories = computed(() => [
    ...(this.first.hasValue() ? (this.first.value()?.items ?? []) : []),
    ...this.extra(),
  ]);
  protected readonly hasMore = computed(
    () =>
      this.extraHasMore() ?? (this.first.hasValue() ? this.first.value()?.has_more : false) ?? false,
  );

  constructor() {
    const seo = inject(Seo);
    effect(() => {
      const label = this.activeLabel();
      seo.describe({
        title: label ? `${label} · News` : 'Football News',
        description:
          'The most important football stories for scouts: transfers, injuries, managers and young talents.',
        path: '/news',
        noindex: !!label,
      });
    });
    effect(() => {
      this.active();
      this.extra.set([]);
      this.extraHasMore.set(null);
      this.moreError.set(null);
    });
  }

  protected async loadMore(): Promise<void> {
    this.loadingMore.set(true);
    this.moreError.set(null);
    try {
      const page = await this.reader.list({
        category: this.active(),
        limit: PAGE,
        offset: this.stories().length,
      });
      // A run between pages can shift the list; never show a story twice.
      const seen = new Set(this.stories().map((s) => s.id));
      this.extra.update((list) => [...list, ...page.items.filter((s) => !seen.has(s.id))]);
      this.extraHasMore.set(page.has_more);
    } catch (e) {
      this.moreError.set(messageFor(e, 'Could not load more stories.'));
    } finally {
      this.loadingMore.set(false);
    }
  }
}

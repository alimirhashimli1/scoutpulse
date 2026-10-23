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

import { ARTICLE_READER } from '../../../core/api/articles';
import { ssrResource } from '../../../core/api/ssr-resource';
import { SessionStore } from '../../../core/auth/session-store';
import { ArticleSummary } from '../../../core/models/articles';
import { Seo } from '../../../core/seo/seo';
import { messageFor } from '../../../shared/forms/submit';
import { Empty, ErrorState, Loading } from '../../../shared/ui/states';
import { ArticleCard } from '../ui/article-card';
import { ArticleIcon } from '../ui/article-icon';

const PAGE = 13; // a lead story and a grid of twelve

/**
 * The analysis section: every live article, newest first, with the latest as
 * the lead story. `?tag=` narrows it to one topic.
 */
@Component({
  selector: 'app-article-list',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, ArticleCard, ArticleIcon, Loading, Empty, ErrorState],
  template: `
    <section class="page section">
      <header class="head">
        <div>
          <p class="eyebrow">ScoutPulse</p>
          <h1>Analysis</h1>
          <p class="lede">
            Tactics, scouting and the ideas behind the game, from the ScoutPulse desk.
          </p>
        </div>
        @if (session.isEditor()) {
          <div class="desk">
            <a class="btn" routerLink="/desk">Editorial desk</a>
            <a class="btn primary" routerLink="/desk/new"><app-article-icon name="pen" /> Write</a>
          </div>
        }
      </header>

      @if (tag(); as t) {
        <p class="filter">
          Showing articles tagged <strong>#{{ t }}</strong>
          <a routerLink="/articles" aria-label="Show all articles"
            ><app-article-icon name="close"
          /></a>
        </p>
      }

      @if (first.value()) {
        @if (articles().length) {
          <div class="lead">
            <app-article-card [article]="articles()[0]" [lead]="true" />
          </div>
          @if (articles().length > 1) {
            <ul class="grid">
              @for (a of articles().slice(1); track a.id) {
                <li><app-article-card [article]="a" /></li>
              }
            </ul>
          }
          @if (hasMore()) {
            <button type="button" class="btn more" [disabled]="loadingMore()" (click)="loadMore()">
              {{ loadingMore() ? 'Loading…' : 'More articles' }}
            </button>
          }
        } @else {
          <app-empty
            [message]="tag() ? 'Nothing tagged #' + tag() + ' yet.' : 'No articles yet.'"
            [hint]="session.isEditor() ? 'Write the first one from the editorial desk.' : null"
          />
        }
      } @else if (first.isLoading()) {
        <app-loading [lines]="6" message="Loading analysis…" />
      } @else if (first.error()) {
        <app-error-state message="Could not load the articles." />
      }
      @if (moreError()) {
        <app-error-state [message]="moreError()!" />
      }
    </section>
  `,
  styles: `
    .section {
      padding-block: var(--space-6) var(--space-8);
      display: flex;
      flex-direction: column;
      gap: var(--space-6);
    }
    .head {
      display: flex;
      justify-content: space-between;
      align-items: flex-end;
      gap: var(--space-4);
      flex-wrap: wrap;
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
    }
    .desk {
      display: flex;
      gap: var(--space-2);
    }
    .desk .btn {
      padding: var(--space-2) var(--space-4);
    }
    .filter {
      display: flex;
      align-items: center;
      gap: var(--space-2);
      color: var(--ink-soft);
    }
    .filter a {
      display: inline-grid;
      place-items: center;
      width: 1.6rem;
      height: 1.6rem;
      border-radius: 50%;
      background: var(--surface-2);
      color: var(--ink-soft);
    }
    .lead {
      padding-bottom: var(--space-6);
      border-bottom: 1px solid var(--line);
    }
    .grid {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(17rem, 1fr));
      gap: var(--space-6) var(--space-5);
    }
    .more {
      align-self: center;
    }
  `,
})
export class ArticleList {
  /** From the query string, via withComponentInputBinding. */
  readonly tag = input<string | undefined>();

  private readonly reader = inject(ARTICLE_READER);
  protected readonly session = inject(SessionStore);

  protected readonly first = ssrResource('articles', {
    params: () => ({ tag: this.tag() || undefined }),
    loader: ({ params }) => this.reader.list({ ...params, limit: PAGE }),
  });

  private readonly extra = signal<ArticleSummary[]>([]);
  private readonly extraHasMore = signal<boolean | null>(null);
  protected readonly loadingMore = signal(false);
  protected readonly moreError = signal<string | null>(null);

  protected readonly articles = computed(() => [
    ...(this.first.value()?.items ?? []),
    ...this.extra(),
  ]);
  protected readonly hasMore = computed(
    () => this.extraHasMore() ?? this.first.value()?.has_more ?? false,
  );

  constructor() {
    const seo = inject(Seo);
    effect(() => {
      const tag = this.tag();
      seo.describe({
        title: tag ? `#${tag} · Analysis` : 'Analysis',
        description: tag
          ? `Football analysis tagged ${tag}, from the ScoutPulse desk.`
          : 'Tactics, scouting and the ideas behind the game, from the ScoutPulse desk.',
        path: '/articles',
        // A tag page is a slice of the section, not a page of its own.
        noindex: !!tag,
      });
    });
    // A new tag starts from its first page.
    effect(() => {
      this.tag();
      this.extra.set([]);
      this.extraHasMore.set(null);
    });
  }

  protected async loadMore(): Promise<void> {
    this.loadingMore.set(true);
    this.moreError.set(null);
    try {
      const page = await this.reader.list({
        tag: this.tag() || undefined,
        limit: PAGE,
        offset: this.articles().length,
      });
      this.extra.update((list) => [...list, ...page.items]);
      this.extraHasMore.set(page.has_more);
    } catch (e) {
      this.moreError.set(messageFor(e, 'Could not load more articles.'));
    } finally {
      this.loadingMore.set(false);
    }
  }
}

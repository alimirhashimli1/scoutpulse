import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import { NewsStory } from '../../core/models/news';
import { NewsIcon } from './news-icon';
import { categoryLabel, importance, timeAgo } from './news-meta';

/**
 * One story: its headline, the summary, who reported it and when, and a link
 * out to read it at the source. The headline links out too -- the outlet's
 * page is the only place the story itself lives.
 */
@Component({
  selector: 'app-news-card',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [NewsIcon],
  template: `
    <article class="card" [class.compact]="compact()" [class.pinned]="story().pinned">
      <div class="thumb" [attr.data-cat]="story().category">
        <app-news-icon [category]="story().category" />
      </div>
      <div class="body">
        <p class="meta">
          <span class="badge" [attr.data-cat]="story().category">{{ label() }}</span>
          <span
            class="importance"
            [class.major]="level().level === 'major'"
            role="img"
            [attr.aria-label]="level().label"
            [title]="level().label"
          >
            @for (i of bars; track i) {
              <span class="bar" [class.on]="story().score >= i"></span>
            }
          </span>
          @if (story().pinned) {
            <span class="pin">Pinned</span>
          }
        </p>
        <h3 class="headline">
          <a [href]="story().url" target="_blank" rel="noopener noreferrer nofollow">{{
            story().headline
          }}<span class="visually-hidden"> (opens {{ story().source_name }} in a new tab)</span></a>
        </h3>
        @if (story().summary && !compact()) {
          <p class="summary">{{ story().summary }}</p>
        }
        <p class="foot">
          <span class="source">{{ story().source_name }}</span>
          @if (story().source_count > 1) {
            <span class="muted">+{{ story().source_count - 1 }} more</span>
          }
          <span class="muted" aria-hidden="true">·</span>
          <time class="muted" [attr.datetime]="story().published_at">{{ ago() }}</time>
          @if (!compact()) {
            <a class="read" [href]="story().url" target="_blank" rel="noopener noreferrer nofollow"
              >Read at {{ story().source_name
              }}<span class="visually-hidden"> (opens in a new tab)</span>
              <span aria-hidden="true">↗</span></a
            >
          }
        </p>
        @if (story().attribution && !compact()) {
          <p class="attribution">{{ story().attribution }}</p>
        }
      </div>
    </article>
  `,
  styles: `
    .card {
      display: grid;
      grid-template-columns: 3.25rem 1fr;
      gap: var(--space-4);
      padding: var(--space-4);
      background: var(--surface);
      border: 1px solid var(--line);
      border-radius: var(--radius-lg);
      height: 100%;
    }
    .card.pinned {
      border-color: var(--accent);
    }
    .card.compact {
      grid-template-columns: 2.5rem 1fr;
      gap: var(--space-3);
      padding: var(--space-3) var(--space-4);
    }
    .thumb {
      width: 100%;
      aspect-ratio: 1;
      border-radius: var(--radius);
      background: var(--accent-soft);
      color: var(--accent);
      display: grid;
      place-items: center;
      padding: 22%;
    }
    .thumb[data-cat='injuries'] {
      background: var(--critical-soft);
      color: var(--critical);
    }
    .thumb[data-cat='managers'] {
      background: var(--warning-soft);
      color: var(--warning);
    }
    .thumb[data-cat='other'],
    .thumb[data-cat='results'] {
      background: var(--surface-2);
      color: var(--ink-soft);
    }
    .thumb app-news-icon {
      width: 100%;
      height: 100%;
    }
    .body {
      min-width: 0;
      display: flex;
      flex-direction: column;
      gap: var(--space-2);
    }
    .meta {
      display: flex;
      align-items: center;
      gap: var(--space-2);
      flex-wrap: wrap;
      font-size: var(--text-xs);
    }
    .badge {
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--accent);
    }
    .badge[data-cat='injuries'] {
      color: var(--critical);
    }
    .badge[data-cat='managers'] {
      color: var(--warning);
    }
    .badge[data-cat='other'],
    .badge[data-cat='results'] {
      color: var(--ink-soft);
    }
    .importance {
      display: inline-flex;
      gap: 2px;
      align-items: flex-end;
    }
    .bar {
      width: 4px;
      height: 10px;
      border-radius: 1px;
      background: var(--line);
    }
    .bar.on {
      background: var(--accent);
    }
    .importance.major .bar.on {
      background: var(--critical);
    }
    .pin {
      font-weight: 700;
      color: var(--accent);
    }
    .headline {
      font-family: var(--font-display);
      font-size: var(--text-lg);
      line-height: var(--leading-tight);
      overflow-wrap: anywhere;
    }
    .compact .headline {
      font-size: var(--text-base);
    }
    .headline a {
      color: var(--ink);
      text-decoration: none;
    }
    .headline a:hover {
      text-decoration: underline;
    }
    .headline a:focus-visible,
    .read:focus-visible {
      outline: 2px solid var(--accent);
      outline-offset: 2px;
      border-radius: var(--radius-sm);
    }
    .summary {
      color: var(--ink-soft);
      font-size: var(--text-sm);
    }
    .foot {
      display: flex;
      align-items: center;
      flex-wrap: wrap;
      gap: var(--space-1) var(--space-2);
      font-size: var(--text-xs);
      margin-top: auto;
    }
    .source {
      font-weight: 600;
      color: var(--ink-soft);
    }
    .muted {
      color: var(--muted);
    }
    .read {
      margin-left: auto;
      font-weight: 600;
      color: var(--accent);
    }
    .attribution {
      font-size: var(--text-xs);
      color: var(--muted);
    }
  `,
})
export class NewsCard {
  readonly story = input.required<NewsStory>();
  readonly compact = input(false);
  /** The clock, so a render and a test agree on "2h ago". */
  readonly now = input<Date>(new Date());

  protected readonly bars = [2, 4, 6, 8, 10];
  protected readonly label = computed(() => categoryLabel(this.story().category));
  protected readonly level = computed(() => importance(this.story().score));
  protected readonly ago = computed(() => timeAgo(this.story().published_at, this.now()));
}

import { ChangeDetectionStrategy, Component, computed, input, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { ArticleSummary } from '../../../core/models/articles';
import { safeImage } from '../doc/article-doc';

/**
 * An article in a list. `lead` is the large first story on the section page.
 *
 * An article without a cover, or whose cover no longer loads, gets a pitch
 * pattern instead, so the grid keeps its rhythm.
 */
@Component({
  selector: 'app-article-card',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, DatePipe],
  template: `
    <a class="card" [class.lead]="lead()" [routerLink]="['/articles', article().slug]">
      <span class="media">
        @if (cover(); as src) {
          <img
            [src]="src"
            alt=""
            loading="lazy"
            referrerpolicy="no-referrer"
            (error)="broken.set(true)"
          />
        } @else {
          <span class="pitch" aria-hidden="true"></span>
        }
      </span>
      <span class="text">
        @if (article().tags[0]; as tag) {
          <span class="kicker">{{ tag }}</span>
        }
        <span class="title">{{ article().title }}</span>
        @if (article().subtitle || article().excerpt; as dek) {
          <span class="dek">{{ dek }}</span>
        }
        <span class="meta">
          {{ article().author_name }} · {{ article().publish_at | date: 'd MMM y' }} ·
          {{ article().reading_minutes }} min read
        </span>
      </span>
    </a>
  `,
  styles: `
    .card {
      display: flex;
      flex-direction: column;
      gap: var(--space-3);
      height: 100%;
      color: var(--ink);
      text-decoration: none;
    }
    .media {
      position: relative;
      display: block;
      aspect-ratio: 16 / 9;
      border-radius: var(--radius-lg);
      overflow: hidden;
      background: var(--surface-2);
    }
    img,
    .pitch {
      position: absolute;
      inset: 0;
      width: 100%;
      height: 100%;
      object-fit: cover;
      transition: transform 0.3s ease;
    }
    .pitch {
      background:
        radial-gradient(
          circle at 50% 50%,
          transparent 17%,
          rgb(255 255 255 / 55%) 17.6%,
          transparent 18.4%
        ),
        linear-gradient(
          90deg,
          transparent 49.6%,
          rgb(255 255 255 / 55%) 49.6% 50.4%,
          transparent 50.4%
        ),
        repeating-linear-gradient(90deg, #3f9b4c 0 10%, #378f45 10% 20%);
    }
    .card:hover img,
    .card:hover .pitch {
      transform: scale(1.03);
    }
    .text {
      display: flex;
      flex-direction: column;
      gap: var(--space-1);
    }
    .kicker {
      font-size: var(--text-xs);
      font-weight: 700;
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: var(--accent);
    }
    .title {
      font-family: var(--font-display);
      font-size: var(--text-xl);
      line-height: var(--leading-tight);
      text-wrap: balance;
    }
    .card:hover .title {
      color: var(--accent);
    }
    .dek {
      color: var(--ink-soft);
      font-size: var(--text-sm);
      display: -webkit-box;
      -webkit-line-clamp: 3;
      -webkit-box-orient: vertical;
      overflow: hidden;
    }
    .meta {
      margin-top: var(--space-1);
      font-size: var(--text-xs);
      color: var(--muted);
    }
    .lead {
      display: grid;
      grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr);
      gap: var(--space-6);
      align-items: center;
    }
    .lead .title {
      font-size: clamp(var(--text-2xl), 3.4vw, var(--text-4xl));
    }
    .lead .dek {
      font-size: var(--text-lg);
      -webkit-line-clamp: 4;
    }
    @media (max-width: 48rem) {
      .lead {
        display: flex;
      }
    }
  `,
})
export class ArticleCard {
  readonly article = input.required<ArticleSummary>();
  readonly lead = input(false);
  protected readonly broken = signal(false);
  protected readonly cover = computed(() =>
    this.broken() ? null : safeImage(this.article().cover_url),
  );
}

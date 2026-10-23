import {
  ChangeDetectionStrategy,
  Component,
  DOCUMENT,
  PLATFORM_ID,
  computed,
  effect,
  inject,
  input,
  signal,
} from '@angular/core';
import { DatePipe, isPlatformBrowser } from '@angular/common';
import { RouterLink } from '@angular/router';

import { ApiError } from '../../../core/api/api-error';
import { ARTICLE_READER } from '../../../core/api/articles';
import { ssrResource } from '../../../core/api/ssr-resource';
import { SessionStore } from '../../../core/auth/session-store';
import { Seo } from '../../../core/seo/seo';
import { SITE_URL } from '../../../core/tokens/site-url';
import { ErrorState, Loading } from '../../../shared/ui/states';
import { ArticleBody } from '../doc/article-body';
import { outline, safeImage } from '../doc/article-doc';
import { ArticleIcon } from '../ui/article-icon';
import { ArticleCard } from '../ui/article-card';

/**
 * One article, as readers get it.
 *
 * Server-rendered and indexed: unlike a tactics board, an article is meant to
 * be found. The server render carries the whole text, the description and
 * cover for link previews, and Article structured data. Embedded boards and
 * videos are deferred to the browser, where they load as the reader reaches
 * them.
 */
@Component({
  selector: 'app-article-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, DatePipe, ArticleBody, ArticleCard, ArticleIcon, Loading, ErrorState],
  templateUrl: './article-page.html',
  styleUrl: './article-page.css',
  host: { '(window:scroll)': 'onScroll()' },
})
export class ArticlePage {
  readonly slug = input.required<string>();

  private readonly reader = inject(ARTICLE_READER);
  private readonly document = inject(DOCUMENT);
  private readonly isBrowser = isPlatformBrowser(inject(PLATFORM_ID));
  private readonly origin = inject(SITE_URL);
  protected readonly session = inject(SessionStore);

  protected readonly article = ssrResource('article', {
    params: () => this.slug(),
    loader: ({ params }) => this.reader.bySlug(params),
  });
  protected readonly more = ssrResource('article-more', {
    params: () => this.slug(),
    loader: async ({ params }) => {
      const page = await this.reader.list({ limit: 4 });
      return page.items.filter((a) => a.slug !== params).slice(0, 3);
    },
  });

  protected readonly cover = computed(() => safeImage(this.article.value()?.cover_url));
  protected readonly contents = computed(() => {
    const a = this.article.value();
    const entries = a ? outline(a.body).entries.filter((e) => e.level === 2) : [];
    // A contents list for one or two sections is clutter, not navigation.
    return entries.length >= 3 ? entries : [];
  });
  protected readonly canEdit = computed(() => {
    const a = this.article.value();
    const me = this.session.user();
    return !!a && !!me && (this.session.isAdmin() || a.author_id === me.id);
  });
  protected readonly updatedLater = computed(() => {
    const a = this.article.value();
    if (!a?.publish_at) return false;
    // Only worth saying once the edit is a day or more after publication.
    return Date.parse(a.updated_at) - Date.parse(a.publish_at) > 24 * 3600 * 1000;
  });
  protected readonly notFound = computed(() => {
    const e = this.article.error();
    return e instanceof ApiError && e.code === 'not_found';
  });

  protected readonly progress = signal(0);
  protected readonly copied = signal(false);

  constructor() {
    const seo = inject(Seo);
    effect(() => {
      const a = this.article.value();
      const path = `/articles/${this.slug()}`;
      seo.describe({
        title: a ? a.title : 'Analysis',
        description: a ? a.subtitle || a.excerpt : 'Football analysis on ScoutPulse.',
        path,
        type: 'article',
        image: this.cover() ?? undefined,
        noindex: !a,
      });
      seo.structuredData(
        a
          ? {
              '@context': 'https://schema.org',
              '@type': 'Article',
              headline: a.title,
              description: a.subtitle || a.excerpt,
              image: this.cover() ? [this.cover()] : undefined,
              datePublished: a.publish_at,
              dateModified: a.updated_at,
              author: { '@type': 'Person', name: a.author_name },
              keywords: a.tags.join(', ') || undefined,
              mainEntityOfPage: `${this.origin}${path}`,
            }
          : null,
      );
    });
  }

  protected onScroll(): void {
    if (!this.isBrowser) return;
    const el = this.document.documentElement;
    const max = el.scrollHeight - el.clientHeight;
    this.progress.set(max > 0 ? Math.min(1, el.scrollTop / max) : 0);
  }

  protected async copyLink(): Promise<void> {
    try {
      await navigator.clipboard.writeText(this.document.location.href);
      this.copied.set(true);
      setTimeout(() => this.copied.set(false), 2000);
    } catch {
      // The address bar has it.
    }
  }

  protected shareOnX(title: string): string {
    const url = `${this.origin}/articles/${this.slug()}`;
    return `https://x.com/intent/post?text=${encodeURIComponent(title)}&url=${encodeURIComponent(url)}`;
  }
}

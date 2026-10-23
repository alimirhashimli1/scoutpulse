import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { ARTICLE_DESK } from '../../../core/api/articles';
import { MAX_PAGE_SIZE } from '../../../core/api/page';
import { SessionStore } from '../../../core/auth/session-store';
import { ArticleSummary, isLive } from '../../../core/models/articles';
import { Seo } from '../../../core/seo/seo';
import { messageFor } from '../../../shared/forms/submit';
import { ConfirmDialog } from '../../../shared/ui/confirm-dialog';
import { Empty, ErrorState, Loading } from '../../../shared/ui/states';
import { ArticleIcon } from '../ui/article-icon';
import { ArticleStatusPill, StatusTone, relativeTime, statusLabel } from '../ui/article-status';

type Tab = 'all' | StatusTone;

const TABS: { value: Tab; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'draft', label: 'Drafts' },
  { value: 'review', label: 'In review' },
  { value: 'changes', label: 'Changes requested' },
  { value: 'scheduled', label: 'Scheduled' },
  { value: 'live', label: 'Published' },
];

/**
 * The editorial desk: every article that is not simply a page for readers.
 *
 * An editor sees their own; the API returns nothing else to them. An
 * administrator sees everyone's, with the review queue -- other writers'
 * submissions -- called out at the top, since that is the part only they can
 * move forward.
 */
@Component({
  selector: 'app-article-desk',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    RouterLink,
    DatePipe,
    ArticleIcon,
    ArticleStatusPill,
    ConfirmDialog,
    Loading,
    Empty,
    ErrorState,
  ],
  templateUrl: './article-desk.html',
  styleUrl: './article-desk.css',
})
export class ArticleDesk {
  private readonly desk = inject(ARTICLE_DESK);
  protected readonly session = inject(SessionStore);

  protected readonly tabs = TABS;
  protected readonly articles = signal<ArticleSummary[]>([]);
  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);
  protected readonly hasMore = signal(false);
  protected readonly tab = signal<Tab>('all');
  protected readonly scope = signal<'everyone' | 'mine'>('everyone');
  protected readonly toDelete = signal<ArticleSummary | null>(null);
  protected readonly confirmingDelete = signal(false);

  private readonly me = computed(() => this.session.user()?.id);

  /** Other writers' submissions, which only an administrator can move on. */
  protected readonly queue = computed(() =>
    this.session.isAdmin()
      ? this.articles().filter((a) => a.status === 'in_review' && a.author_id !== this.me())
      : [],
  );

  private readonly scoped = computed(() =>
    this.scope() === 'mine'
      ? this.articles().filter((a) => a.author_id === this.me())
      : this.articles(),
  );

  protected readonly counts = computed(() => {
    const counts: Record<Tab, number> = {
      all: 0,
      draft: 0,
      review: 0,
      changes: 0,
      scheduled: 0,
      live: 0,
    };
    for (const a of this.scoped()) {
      counts.all++;
      counts[statusLabel(a).tone]++;
    }
    return counts;
  });

  protected readonly visible = computed(() => {
    const tab = this.tab();
    return tab === 'all' ? this.scoped() : this.scoped().filter((a) => statusLabel(a).tone === tab);
  });

  constructor() {
    inject(Seo).describe({
      title: 'Editorial desk',
      description: 'Drafts, reviews and scheduled analysis.',
      path: '/desk',
      noindex: true,
    });
    void this.load();
  }

  protected async load(): Promise<void> {
    this.loading.set(true);
    this.error.set(null);
    try {
      const page = await this.desk.list({ limit: MAX_PAGE_SIZE, offset: this.articles().length });
      this.articles.update((list) => [...list, ...page.items]);
      this.hasMore.set(page.has_more);
    } catch (e) {
      this.error.set(messageFor(e, 'Could not load the desk.'));
    } finally {
      this.loading.set(false);
    }
  }

  protected isMine(a: ArticleSummary): boolean {
    return a.author_id === this.me();
  }

  protected live(a: ArticleSummary): boolean {
    return isLive(a);
  }

  protected ago(iso: string | null): string {
    return relativeTime(iso);
  }

  /** Whether this person may delete it: the API's rule, mirrored. */
  protected canDelete(a: ArticleSummary): boolean {
    return this.session.isAdmin() || (this.isMine(a) && a.status !== 'published');
  }

  protected askDelete(a: ArticleSummary): void {
    this.toDelete.set(a);
    this.confirmingDelete.set(true);
  }

  protected async confirmDelete(): Promise<void> {
    const a = this.toDelete();
    if (!a) return;
    try {
      await this.desk.remove(a.id);
      this.articles.update((list) => list.filter((x) => x.id !== a.id));
    } catch (e) {
      this.error.set(messageFor(e, 'Could not delete the article.'));
    } finally {
      this.toDelete.set(null);
    }
  }
}

import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { MAX_PAGE_SIZE } from '../../../core/api/page';
import { TACTIC_READER, TACTIC_WRITER } from '../../../core/api/tactics';
import { SessionStore } from '../../../core/auth/session-store';
import { TacticSummary } from '../../../core/models/tactics';
import { Seo } from '../../../core/seo/seo';
import { messageFor } from '../../../shared/forms/submit';
import { ConfirmDialog } from '../../../shared/ui/confirm-dialog';
import { Empty, ErrorState, Loading } from '../../../shared/ui/states';
import { TacticIcon } from '../ui/icon';

interface AuthorGroup {
  ownerId: string;
  name: string;
  boards: TacticSummary[];
  mine: boolean;
}

/**
 * Every member's tactics boards, one section per author.
 *
 * Members only: the boards themselves are watchable by link, but this
 * directory is what would make them findable, so it sits behind sign-in. An
 * author appears only once they have made a board — the grouping comes from
 * the boards, not from a list of accounts.
 *
 * The signed-in member's own section comes first, since it is the one they
 * act on.
 */
@Component({
  selector: 'app-tactics-list',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, DatePipe, TacticIcon, Loading, Empty, ErrorState, ConfirmDialog],
  templateUrl: './tactics-list.html',
  styleUrl: './tactics-list.css',
})
export class TacticsList {
  private readonly reader = inject(TACTIC_READER);
  private readonly writer = inject(TACTIC_WRITER);
  protected readonly session = inject(SessionStore);

  protected readonly boards = signal<TacticSummary[]>([]);
  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);
  protected readonly hasMore = signal(false);
  protected readonly filter = signal<'all' | 'mine'>('all');
  protected readonly confirmingDelete = signal(false);
  protected readonly toDelete = signal<TacticSummary | null>(null);

  protected readonly groups = computed<AuthorGroup[]>(() => {
    const me = this.session.user()?.id;
    const byOwner = new Map<string, AuthorGroup>();
    for (const b of this.boards()) {
      if (this.filter() === 'mine' && b.owner_id !== me) continue;
      let g = byOwner.get(b.owner_id);
      if (!g) {
        g = {
          ownerId: b.owner_id,
          name: b.owner_name || 'A member',
          boards: [],
          mine: b.owner_id === me,
        };
        byOwner.set(b.owner_id, g);
      }
      g.boards.push(b);
    }
    // The API already orders by author; only "mine first" is applied here.
    return [...byOwner.values()].sort((a, b) => Number(b.mine) - Number(a.mine));
  });

  protected readonly myCount = computed(
    () => this.boards().filter((b) => b.owner_id === this.session.user()?.id).length,
  );

  constructor() {
    inject(Seo).describe({
      title: 'Tactics boards',
      description: "Members' animated drills and match analysis.",
      path: '/tactics',
      noindex: true,
    });
    void this.loadMore();
  }

  protected async loadMore(): Promise<void> {
    this.loading.set(true);
    this.error.set(null);
    try {
      const page = await this.reader.list({ limit: MAX_PAGE_SIZE, offset: this.boards().length });
      this.boards.update((b) => [...b, ...page.items]);
      this.hasMore.set(page.has_more);
    } catch (e) {
      this.error.set(messageFor(e, 'Could not load the boards.'));
    } finally {
      this.loading.set(false);
    }
  }

  protected askDelete(b: TacticSummary): void {
    this.toDelete.set(b);
    this.confirmingDelete.set(true);
  }

  protected async confirmDelete(): Promise<void> {
    const b = this.toDelete();
    if (!b) return;
    try {
      await this.writer.remove(b.slug);
      this.boards.update((list) => list.filter((x) => x.slug !== b.slug));
    } catch (e) {
      this.error.set(messageFor(e, 'Could not delete the board.'));
    } finally {
      this.toDelete.set(null);
    }
  }
}

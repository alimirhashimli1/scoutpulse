import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { Router, RouterLink } from '@angular/router';

import { MAX_PAGE_SIZE } from '../../../core/api/page';
import { TACTIC_READER, TACTIC_WRITER } from '../../../core/api/tactics';
import { SessionStore } from '../../../core/auth/session-store';
import { TacticSummary } from '../../../core/models/tactics';
import { Seo } from '../../../core/seo/seo';
import { messageFor } from '../../../shared/forms/submit';
import { ConfirmDialog } from '../../../shared/ui/confirm-dialog';
import { Empty, ErrorState, Loading } from '../../../shared/ui/states';
import { TacticIcon } from '../ui/icon';

/** Matches the API's limit on a board title. */
const MAX_TITLE_LENGTH = 120;

interface AuthorGroup {
  ownerId: string;
  name: string;
  boards: TacticSummary[];
  mine: boolean;
}

/**
 * The signed-in member's tactics boards.
 *
 * The API returns only the caller's own boards -- a list of everyone's, open to
 * any account, would be open to anyone who signs up, and the boards are meant to
 * be reached only by the link their author sends. An administrator gets every
 * member's, one section per author, with the All / Mine filter to narrow it.
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
  private readonly router = inject(Router);
  protected readonly session = inject(SessionStore);

  protected readonly boards = signal<TacticSummary[]>([]);
  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);
  protected readonly hasMore = signal(false);
  protected readonly filter = signal<'all' | 'mine'>('all');
  protected readonly confirmingDelete = signal(false);
  protected readonly toDelete = signal<TacticSummary | null>(null);
  /** Slug of the board being copied, so its button can show progress. */
  protected readonly duplicating = signal<string | null>(null);

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

  /**
   * Copies a board -- every step, piece and drawing -- into a new board owned
   * by the caller, then opens the copy in the editor so the steps that differ
   * can be changed while the rest stay as they were.
   */
  protected async duplicate(b: TacticSummary): Promise<void> {
    if (this.duplicating()) return;
    this.duplicating.set(b.slug);
    this.error.set(null);
    try {
      const source = await this.reader.bySlug(b.slug);
      const suffix = ' (copy)';
      const copy = await this.writer.create({
        title: source.title.slice(0, MAX_TITLE_LENGTH - suffix.length) + suffix,
        description: source.description,
        content: structuredClone(source.content),
      });
      await this.router.navigate(['/tactics', copy.slug, 'edit']);
    } catch (e) {
      this.error.set(messageFor(e, 'Could not duplicate the board.'));
    } finally {
      this.duplicating.set(null);
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

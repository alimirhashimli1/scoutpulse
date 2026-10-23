import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  input,
  output,
  signal,
} from '@angular/core';
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { Article, isLive, isScheduled } from '../../../core/models/articles';
import { ArticleIcon } from '../ui/article-icon';
import {
  ArticleStatusPill,
  fromLocalInput,
  relativeTime,
  toLocalInput,
} from '../ui/article-status';

const MAX_TAGS = 8;

/**
 * The editor's side panel: where the article is in review, what can be done
 * next, when it goes live, and its tags.
 *
 * It offers only what the API will accept from this person -- an editor
 * submits, an administrator publishes, approves or sends back -- and says in
 * plain words what each one will do, since "submit" and "publish" are easy to
 * confuse when one of them needs somebody else.
 */
@Component({
  selector: 'app-publish-panel',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DatePipe, RouterLink, ArticleIcon, ArticleStatusPill],
  templateUrl: './publish-panel.html',
  styleUrl: './publish-panel.css',
})
export class PublishPanel {
  readonly article = input<Article | null>(null);
  readonly isAdmin = input(false);
  readonly myId = input<string | undefined>(undefined);
  readonly tags = input<string[]>([]);
  readonly busy = input(false);
  readonly words = input(0);
  readonly canEdit = input(true);

  readonly tagsChange = output<string[]>();
  readonly submit = output<string | null>();
  readonly requestChanges = output<string>();
  readonly withdraw = output<void>();
  readonly remove = output<void>();

  protected readonly when = signal<'now' | 'later'>('now');
  protected readonly at = signal('');
  protected readonly tagDraft = signal('');
  protected readonly reviewing = signal(false);
  protected readonly note = signal('');

  protected readonly mine = computed(() => {
    const a = this.article();
    return !a || a.author_id === this.myId();
  });
  protected readonly status = computed(() => this.article()?.status ?? 'draft');
  protected readonly live = computed(() => {
    const a = this.article();
    return !!a && isLive(a);
  });
  protected readonly scheduled = computed(() => {
    const a = this.article();
    return !!a && isScheduled(a);
  });
  /** An administrator looking at someone else's submission. */
  protected readonly reviewingOther = computed(
    () => this.isAdmin() && !this.mine() && this.status() === 'in_review',
  );
  protected readonly minAt = computed(() =>
    toLocalInput(new Date(Date.now() + 60_000).toISOString()),
  );
  protected readonly publishAt = computed(() =>
    this.when() === 'later' ? fromLocalInput(this.at()) : null,
  );
  protected readonly atInvalid = computed(
    () =>
      this.when() === 'later' && (!this.publishAt() || Date.parse(this.publishAt()!) <= Date.now()),
  );
  protected readonly minutes = computed(() => Math.max(1, Math.ceil(this.words() / 230)));

  protected readonly primaryLabel = computed(() => {
    const later = this.when() === 'later';
    if (!this.isAdmin()) return later ? 'Submit with this date' : 'Submit for review';
    if (this.reviewingOther()) return later ? 'Approve and schedule' : 'Approve and publish';
    if (this.scheduled()) return later ? 'Reschedule' : 'Publish now';
    return later ? 'Schedule' : 'Publish now';
  });

  constructor() {
    // A requested or scheduled time opens the picker on it.
    effect(() => {
      const a = this.article();
      if (a?.publish_at && Date.parse(a.publish_at) > Date.now()) {
        this.when.set('later');
        this.at.set(toLocalInput(a.publish_at));
      }
    });
  }

  protected ago(iso: string | null): string {
    return relativeTime(iso);
  }

  protected value(e: Event): string {
    return (e.target as HTMLInputElement).value;
  }

  protected go(): void {
    if (this.atInvalid()) return;
    this.submit.emit(this.publishAt());
  }

  protected sendBack(): void {
    const note = this.note().trim();
    if (!note) return;
    this.requestChanges.emit(note);
    this.reviewing.set(false);
    this.note.set('');
  }

  // --- tags -----------------------------------------------------------------

  protected onTagKey(e: KeyboardEvent): void {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      this.addTag();
    } else if (e.key === 'Backspace' && !this.tagDraft() && this.tags().length) {
      this.tagsChange.emit(this.tags().slice(0, -1));
    }
  }

  protected addTag(): void {
    const tag = this.tagDraft()
      .replace(/^#/, '')
      .replace(/\s+/g, ' ')
      .trim()
      .toLowerCase()
      .slice(0, 32);
    this.tagDraft.set('');
    if (!tag || this.tags().includes(tag) || this.tags().length >= MAX_TAGS) return;
    this.tagsChange.emit([...this.tags(), tag]);
  }

  protected removeTag(tag: string): void {
    this.tagsChange.emit(this.tags().filter((t) => t !== tag));
  }

  protected readonly maxTags = MAX_TAGS;
}

import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import { ArticleStatus, isScheduled } from '../../../core/models/articles';

export type StatusTone = 'draft' | 'review' | 'changes' | 'scheduled' | 'live';

/** What the desk calls an article's state. Scheduled is published-but-not-yet. */
export function statusLabel(
  a: { status: ArticleStatus; publish_at: string | null },
  now = Date.now(),
): { label: string; tone: StatusTone } {
  switch (a.status) {
    case 'draft':
      return { label: 'Draft', tone: 'draft' };
    case 'in_review':
      return { label: 'In review', tone: 'review' };
    case 'changes_requested':
      return { label: 'Changes requested', tone: 'changes' };
    case 'published':
      return isScheduled(a, now)
        ? { label: 'Scheduled', tone: 'scheduled' }
        : { label: 'Published', tone: 'live' };
  }
}

@Component({
  selector: 'app-article-status',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `<span [class]="'pill ' + state().tone">{{ state().label }}</span>`,
  styles: `
    .pill {
      display: inline-flex;
      align-items: center;
      gap: 5px;
      padding: 2px 9px 2px 7px;
      border-radius: 999px;
      font-size: var(--text-xs);
      font-weight: 600;
      white-space: nowrap;
      background: var(--surface-2);
      color: var(--ink-soft);
    }
    .pill::before {
      content: '';
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: currentColor;
    }
    .review {
      background: var(--warning-soft);
      color: var(--warning);
    }
    .changes {
      background: var(--critical-soft);
      color: var(--critical);
    }
    .scheduled {
      background: var(--accent-soft);
      color: var(--accent);
    }
    .live {
      background: var(--positive-soft);
      color: var(--positive);
    }
    .live::before {
      box-shadow: 0 0 0 3px color-mix(in srgb, currentColor 25%, transparent);
    }
  `,
})
export class ArticleStatusPill {
  readonly status = input.required<ArticleStatus>();
  readonly publishAt = input<string | null>(null);
  protected readonly state = computed(() =>
    statusLabel({ status: this.status(), publish_at: this.publishAt() }),
  );
}

const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });
const STEPS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['week', 7 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
];

/** "3 hours ago", "in 2 days", "just now". */
export function relativeTime(iso: string | null | undefined, now = Date.now()): string {
  if (!iso) return '';
  const seconds = (Date.parse(iso) - now) / 1000;
  for (const [unit, size] of STEPS) {
    if (Math.abs(seconds) >= size) return rtf.format(Math.round(seconds / size), unit);
  }
  return 'just now';
}

/** `<input type="datetime-local">` speaks local wall-clock time without a zone. */
export function toLocalInput(iso: string | null | undefined): string {
  if (!iso) return '';
  const d = new Date(iso);
  const pad = (n: number) => `${n}`.padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** And back: the wall-clock time the writer picked, as an instant. */
export function fromLocalInput(value: string): string | null {
  if (!value) return null;
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? null : d.toISOString();
}

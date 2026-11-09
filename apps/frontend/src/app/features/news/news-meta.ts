import { NewsCategory } from '../../core/models/news';

/** The six categories, in the order the filter shows them. */
export const NEWS_CATEGORIES: { value: NewsCategory; label: string }[] = [
  { value: 'transfers', label: 'Transfers' },
  { value: 'injuries', label: 'Injuries' },
  { value: 'results', label: 'Results' },
  { value: 'managers', label: 'Managers' },
  { value: 'young_talents', label: 'Young Talents' },
  { value: 'other', label: 'Other' },
];

export function categoryLabel(c: NewsCategory): string {
  return NEWS_CATEGORIES.find((x) => x.value === c)?.label ?? 'Other';
}

export function isCategory(v: string | undefined | null): v is NewsCategory {
  return NEWS_CATEGORIES.some((c) => c.value === v);
}

/**
 * "just now", "5m ago", "2h ago", "3d ago", then a date. Pure, with the clock
 * passed in, so a server render and a test can both pin it.
 */
export function timeAgo(iso: string, now: Date = new Date()): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return '';
  const secs = Math.max(0, Math.round((now.getTime() - then) / 1000));
  if (secs < 60) return 'just now';
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d ago`;
  return new Date(iso).toLocaleDateString('en-GB', { day: 'numeric', month: 'short' });
}

/** The importance band a score falls in, for the indicator and its label. */
export function importance(score: number): { level: 'major' | 'high'; label: string } {
  return score >= 9
    ? { level: 'major', label: `Major story, importance ${score} of 10` }
    : { level: 'high', label: `Important story, importance ${score} of 10` };
}

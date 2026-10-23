import type { ArticleDoc } from '../../features/articles/doc/article-doc';

/** Where an article is in review. "Scheduled" is published with a future publish_at. */
export type ArticleStatus = 'draft' | 'in_review' | 'changes_requested' | 'published';

/** An analysis article as the API returns it. */
export interface Article {
  id: string;
  slug: string;
  author_id: string;
  author_name: string;
  title: string;
  subtitle: string;
  cover_url: string;
  cover_caption: string;
  body: ArticleDoc;
  excerpt: string;
  reading_minutes: number;
  tags: string[];
  status: ArticleStatus;
  publish_at: string | null;
  first_published_at: string | null;
  submitted_at: string | null;
  reviewer_id: string | null;
  reviewer_name: string;
  reviewed_at: string | null;
  review_note: string;
  created_at: string;
  updated_at: string;
}

/** An article in a list: no body. */
export interface ArticleSummary {
  id: string;
  slug: string;
  author_id: string;
  author_name: string;
  title: string;
  subtitle: string;
  cover_url: string;
  excerpt: string;
  reading_minutes: number;
  tags: string[];
  status: ArticleStatus;
  publish_at: string | null;
  submitted_at: string | null;
  review_note: string;
  created_at: string;
  updated_at: string;
}

/** True when readers can see it now. */
export function isLive(
  a: Pick<ArticleSummary, 'status' | 'publish_at'>,
  now = Date.now(),
): boolean {
  return a.status === 'published' && !!a.publish_at && Date.parse(a.publish_at) <= now;
}

/** True when it is published but waiting for its time. */
export function isScheduled(
  a: Pick<ArticleSummary, 'status' | 'publish_at'>,
  now = Date.now(),
): boolean {
  return a.status === 'published' && !!a.publish_at && Date.parse(a.publish_at) > now;
}

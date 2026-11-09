/**
 * Football news, as football-svc serves it.
 *
 * A story is a headline, a short summary ScoutPulse's scorer wrote, the
 * outlet that reported it and a link to read it there. There is never an
 * article body or an image: the outlet's page is where the story is read.
 */

export type NewsCategory =
  | 'transfers'
  | 'injuries'
  | 'results'
  | 'managers'
  | 'young_talents'
  | 'other';

export interface NewsStory {
  readonly id: string;
  readonly headline: string;
  readonly url: string;
  readonly source_name: string;
  readonly attribution: string;
  /** Empty when the rule scorer scored the story: it writes no prose. */
  readonly summary: string;
  readonly category: NewsCategory;
  /** Importance, 1 to 10. Only 7 and above is ever published. */
  readonly score: number;
  readonly source_count: number;
  readonly published_at: string;
  readonly pinned: boolean;
}

/** A story as the admin view sees it, published or not. */
export interface AdminNewsStory extends Omit<NewsStory, 'published_at'> {
  readonly published_at: string | null;
  readonly first_seen_at: string;
  readonly reason: string;
  readonly scorer: 'ai' | 'rules';
  readonly hidden: boolean;
  readonly sources: string[];
}

export interface NewsRun {
  readonly id: number;
  readonly status: 'running' | 'ok' | 'partial' | 'failed';
  readonly fetched: number;
  readonly new_items: number;
  readonly scored: number;
  readonly published: number;
  readonly pruned: number;
  readonly ai_neurons: number;
  readonly errors: string[];
  readonly started_at: string;
  readonly skipped?: string;
}

export interface NewsSourceInfo {
  readonly key: string;
  readonly name: string;
  readonly enabled: boolean;
  readonly commercial_use_verified: boolean;
  readonly terms_url: string;
}

export interface NewsAdminOverview {
  readonly items: AdminNewsStory[];
  readonly has_more: boolean;
  readonly runs: NewsRun[];
  readonly sources: NewsSourceInfo[];
}

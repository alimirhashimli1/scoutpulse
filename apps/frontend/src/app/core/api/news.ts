import { InjectionToken } from '@angular/core';

import { Page, PageQuery } from './page';
import { AdminNewsStory, NewsAdminOverview, NewsCategory, NewsRun, NewsStory } from '../models/news';

/**
 * Football news: the public feed, and its moderation. Two contracts because
 * they are two audiences, as with articles.
 */

export interface NewsFilter extends PageQuery {
  category?: NewsCategory;
}

export interface NewsReader {
  /** Published stories, pinned first, then newest. */
  list(filter?: NewsFilter): Promise<Page<NewsStory>>;
  /** The best few from the last 24 hours, pinned first. */
  top(limit?: number): Promise<NewsStory[]>;
}

export interface NewsAdmin {
  overview(query?: PageQuery): Promise<NewsAdminOverview>;
  setFlags(id: string, flags: { hidden?: boolean; pinned?: boolean }): Promise<AdminNewsStory>;
  runNow(): Promise<NewsRun>;
}

export const NEWS_READER = new InjectionToken<NewsReader>('NewsReader');
export const NEWS_ADMIN = new InjectionToken<NewsAdmin>('NewsAdmin');

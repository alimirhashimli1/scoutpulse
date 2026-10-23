import { InjectionToken } from '@angular/core';

import { Page, PageQuery } from './page';
import { Article, ArticleStatus, ArticleSummary } from '../models/articles';
import type { ArticleDoc } from '../../features/articles/doc/article-doc';

/**
 * Analysis articles: the public section, and the editorial desk behind it.
 *
 * Two contracts because they are two audiences. Anyone reads live articles by
 * slug; only editors and administrators reach the desk, where articles are
 * addressed by id in every state from draft to scheduled.
 */

export interface ArticleFilter extends PageQuery {
  tag?: string;
}

export interface ArticleReader {
  /** A live article. A draft or a scheduled one answers not found. */
  bySlug(slug: string): Promise<Article>;
  list(filter?: ArticleFilter): Promise<Page<ArticleSummary>>;
}

export interface DeskFilter extends PageQuery {
  status?: ArticleStatus;
  /** Narrows an administrator's desk to their own. */
  mine?: boolean;
}

export interface ArticleInput {
  title: string;
  subtitle: string;
  cover_url: string;
  cover_caption: string;
  body: ArticleDoc;
  tags: string[];
}

export interface ArticleDesk {
  list(filter?: DeskFilter): Promise<Page<ArticleSummary>>;
  byId(id: string): Promise<Article>;
  create(input: ArticleInput): Promise<Article>;
  update(id: string, input: ArticleInput): Promise<Article>;
  remove(id: string): Promise<void>;
  /**
   * An editor's: send for review. An administrator's: publish, or approve
   * someone else's. A future `publishAt` schedules it.
   */
  submit(id: string, publishAt: string | null): Promise<Article>;
  requestChanges(id: string, note: string): Promise<Article>;
  /** Back to draft, from review or from published. */
  withdraw(id: string): Promise<Article>;
}

export const ARTICLE_READER = new InjectionToken<ArticleReader>('ArticleReader');
export const ARTICLE_DESK = new InjectionToken<ArticleDesk>('ArticleDesk');

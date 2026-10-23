import { Injectable } from '@angular/core';

import { Page } from '../page';
import { ArticleDesk, ArticleFilter, ArticleInput, ArticleReader, DeskFilter } from '../articles';
import { Article, ArticleSummary } from '../../models/articles';
import { HttpRepository } from './http-repository';

/**
 * Two classes rather than one: both contracts have a `list`, and they mean
 * different endpoints. One class answering both would have to guess which was
 * meant from the filter it was given.
 */
@Injectable()
export class HttpArticleReader extends HttpRepository implements ArticleReader {
  protected readonly base = this.api.football;

  bySlug(slug: string): Promise<Article> {
    return this.getOne<Article>(this.url('articles', slug));
  }

  list(filter?: ArticleFilter): Promise<Page<ArticleSummary>> {
    return this.getPage<ArticleSummary>(this.url('articles'), filter);
  }
}

@Injectable()
export class HttpArticleDesk extends HttpRepository implements ArticleDesk {
  protected readonly base = this.api.football;

  list(filter?: DeskFilter): Promise<Page<ArticleSummary>> {
    return this.getPage<ArticleSummary>(this.url('desk', 'articles'), filter);
  }

  byId(id: string): Promise<Article> {
    return this.getOne<Article>(this.url('desk', 'articles', id));
  }

  create(input: ArticleInput): Promise<Article> {
    return this.post<Article>(this.url('desk', 'articles'), input);
  }

  update(id: string, input: ArticleInput): Promise<Article> {
    return this.put<Article>(this.url('desk', 'articles', id), input);
  }

  remove(id: string): Promise<void> {
    return this.deleteAt(this.url('desk', 'articles', id));
  }

  submit(id: string, publishAt: string | null): Promise<Article> {
    return this.post<Article>(this.url('desk', 'articles', id, 'submit'), {
      publish_at: publishAt,
    });
  }

  requestChanges(id: string, note: string): Promise<Article> {
    return this.post<Article>(this.url('desk', 'articles', id, 'request-changes'), { note });
  }

  withdraw(id: string): Promise<Article> {
    return this.post<Article>(this.url('desk', 'articles', id, 'withdraw'), {});
  }
}

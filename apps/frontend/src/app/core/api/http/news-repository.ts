import { Injectable } from '@angular/core';
import { firstValueFrom } from 'rxjs';

import { NewsAdmin, NewsFilter, NewsReader } from '../news';
import { Page, PageQuery } from '../page';
import { AdminNewsStory, NewsAdminOverview, NewsRun, NewsStory } from '../../models/news';
import { HttpRepository } from './http-repository';

@Injectable()
export class HttpNewsReader extends HttpRepository implements NewsReader {
  protected readonly base = this.api.football;

  list(filter?: NewsFilter): Promise<Page<NewsStory>> {
    return this.getPage<NewsStory>(this.url('news'), filter);
  }

  async top(limit = 5): Promise<NewsStory[]> {
    const res = await this.getOne<{ items: NewsStory[] }>(this.url('news', 'top'), { limit });
    return res.items ?? [];
  }
}

@Injectable()
export class HttpNewsAdmin extends HttpRepository implements NewsAdmin {
  protected readonly base = this.api.football;

  overview(query?: PageQuery): Promise<NewsAdminOverview> {
    return this.getOne<NewsAdminOverview>(this.url('admin', 'news'), query);
  }

  setFlags(id: string, flags: { hidden?: boolean; pinned?: boolean }): Promise<AdminNewsStory> {
    return firstValueFrom(this.http.patch<AdminNewsStory>(this.url('admin', 'news', id), flags));
  }

  runNow(): Promise<NewsRun> {
    return this.post<NewsRun>(this.url('admin', 'news', 'run'), {});
  }
}

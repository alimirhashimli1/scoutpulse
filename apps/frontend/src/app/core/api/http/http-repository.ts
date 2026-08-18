import { HttpClient, HttpParams } from '@angular/common/http';
import { inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';

import { API_CONFIG } from '../../tokens/api-config';
import { Page, PageQuery } from '../page';

/**
 * Shared plumbing for the HTTP repositories.
 *
 * This is the open/closed mechanism from §4: adding a resource means a subclass
 * and a token binding, not edits to anything that already exists. It holds URL
 * building and parameter serialisation — everything a resource shares — and
 * nothing about any particular resource.
 *
 * Errors are not caught here. `errorInterceptor` has already turned them into
 * ApiError by this point, and catching them again would only bury the type.
 */
export abstract class HttpRepository {
  protected readonly http = inject(HttpClient);
  protected readonly api = inject(API_CONFIG);

  /** Base for this repository's service: `api.football` or `api.identity`. */
  protected abstract readonly base: string;

  protected url(...segments: (string | number)[]): string {
    return `${this.base}/api/v1/${segments.map(encodeURIComponent).join('/')}`;
  }

  /**
   * Serialises a query object, dropping anything unset.
   *
   * Undefined and empty values are omitted rather than sent blank: the API
   * treats a present-but-empty filter differently from an absent one in
   * several places, and `?team_id=` is not the same request as no `team_id`.
   */
  protected params(query?: object): HttpParams {
    let params = new HttpParams();
    if (!query) return params;

    for (const [key, value] of Object.entries(query)) {
      if (value === undefined || value === null || value === '') continue;
      params = params.set(key, String(value));
    }
    return params;
  }

  protected getPage<T>(url: string, query?: object): Promise<Page<T>> {
    return firstValueFrom(this.http.get<Page<T>>(url, { params: this.params(query) }));
  }

  protected getOne<T>(url: string, query?: object): Promise<T> {
    return firstValueFrom(this.http.get<T>(url, { params: this.params(query) }));
  }

  protected post<T>(url: string, body: unknown): Promise<T> {
    return firstValueFrom(this.http.post<T>(url, body));
  }

  protected put<T>(url: string, body: unknown): Promise<T> {
    return firstValueFrom(this.http.put<T>(url, body));
  }

  /**
   * DELETE returns 204 with no body throughout this API.
   *
   * Named `deleteAt` rather than `remove` so it does not collide with the
   * `remove(id)` the repository contracts declare — the contract name is the
   * public one and wins.
   */
  protected async deleteAt(url: string): Promise<void> {
    await firstValueFrom(this.http.delete(url, { responseType: 'text' }));
  }
}

/** Convenience for the common list-with-paging call. */
export type ListQuery<F extends PageQuery> = F | undefined;

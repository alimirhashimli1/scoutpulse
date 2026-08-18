/**
 * The envelope every list endpoint returns.
 *
 * Not a bare array — the backend wraps every collection, so this type is built
 * first and every list component consumes it. Retrofitting it later would
 * touch every list in the app.
 */
export interface Page<T> {
  readonly items: T[];
  readonly limit: number;
  readonly offset: number;
  /** Whether another page exists. Computed server-side without a COUNT. */
  readonly has_more: boolean;
}

/** Paging bounds, mirroring the server's. Asking for more is clamped there. */
export const DEFAULT_PAGE_SIZE = 25;
export const MAX_PAGE_SIZE = 100;

/** Query parameters shared by every list endpoint. */
export interface PageQuery {
  limit?: number;
  offset?: number;
}

/** An empty page, for initial signal state before the first request lands. */
export function emptyPage<T>(limit = DEFAULT_PAGE_SIZE): Page<T> {
  return { items: [], limit, offset: 0, has_more: false };
}

/**
 * The page after this one, or null at the end.
 *
 * Offset paging rather than cursors, because that is what the API exposes.
 * It can skip or repeat a row if the data changes between pages — acceptable
 * for browsing, and worth knowing before building an infinite scroll on it.
 */
export function nextPage<T>(page: Page<T>): PageQuery | null {
  return page.has_more ? { limit: page.limit, offset: page.offset + page.limit } : null;
}

export function previousPage<T>(page: Page<T>): PageQuery | null {
  if (page.offset <= 0) return null;
  return { limit: page.limit, offset: Math.max(0, page.offset - page.limit) };
}

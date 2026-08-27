import { Injectable, inject, signal } from '@angular/core';

import { PLAYER_READER } from './contracts';
import { MAX_PAGE_SIZE } from './page';

/**
 * Resolves player ids to names, a page at a time.
 *
 * The transfer feed needs this and `LookupStore` cannot serve it. That store
 * loads *every* club and competition once, which works because both are small.
 * Players are not: a real dataset has hundreds of thousands, so loading them
 * all is out of the question and the trade has to be made differently.
 *
 * So this resolves only the ids actually on screen, in **one** request via
 * `?ids=`, and remembers what it has seen. Paging back to a row already
 * rendered costs nothing; a fresh page costs a single request whatever its
 * length.
 *
 * The alternative — `GET /players/{id}` per row — is twenty-five requests for
 * a twenty-five row feed, every one of them during the server render of the
 * landing page. That is the N+1 this exists to avoid, and it is why the fix
 * needed the API to grow a batch filter rather than being solved here alone.
 */
@Injectable({ providedIn: 'root' })
export class PlayerNames {
  private readonly reader = inject(PLAYER_READER);

  private readonly namesById = signal(new Map<string, string>());

  /** In-flight batches, so two components asking at once do not both fetch. */
  private inFlight = new Map<string, Promise<void>>();

  readonly names = this.namesById.asReadonly();

  name(id: string | null | undefined, fallback = 'Unknown player'): string {
    if (!id) return fallback;
    return this.namesById().get(id) ?? fallback;
  }

  /**
   * Loads any of these ids that are not already known.
   *
   * Safe to call with the same ids repeatedly — a page that re-renders does
   * not re-request. Failures are swallowed: an unresolved name renders as the
   * fallback, which is a worse row but not a broken page.
   */
  async resolve(ids: readonly (string | null | undefined)[]): Promise<void> {
    const known = this.namesById();
    const wanted = [
      ...new Set(
        ids.filter((id): id is string => !!id && !known.has(id) && !this.inFlight.has(id)),
      ),
    ];
    if (wanted.length === 0) return;

    // Chunked because the API caps a batch at one page. A feed page cannot
    // exceed that today, but a caller resolving several pages at once could.
    const batches: string[][] = [];
    for (let i = 0; i < wanted.length; i += MAX_PAGE_SIZE) {
      batches.push(wanted.slice(i, i + MAX_PAGE_SIZE));
    }

    await Promise.all(batches.map((batch) => this.fetch(batch)));
  }

  private fetch(batch: string[]): Promise<void> {
    const request = this.load(batch).finally(() => {
      for (const id of batch) this.inFlight.delete(id);
    });

    for (const id of batch) this.inFlight.set(id, request);
    return request;
  }

  private async load(batch: string[]): Promise<void> {
    try {
      const page = await this.reader.list({ ids: batch, limit: MAX_PAGE_SIZE });
      this.namesById.update((current) => {
        const next = new Map(current);
        for (const player of page.items) next.set(player.id, player.name);
        return next;
      });
    } catch {
      // Leave the ids unresolved. They render as the fallback and will be
      // retried the next time something asks for them.
    }
  }
}

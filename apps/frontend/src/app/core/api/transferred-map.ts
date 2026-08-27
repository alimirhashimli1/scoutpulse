import { PLATFORM_ID, TransferState, inject, makeStateKey } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

/**
 * Carries a lookup map from the server render into the browser.
 *
 * The stores these back — clubs by id, player names by id — are what turn the
 * ids in an API response into the words on the page. They are filled by async
 * loads, so in the browser they begin empty and fill a tick later.
 *
 * On a server-rendered page that is not merely a flash of dashes. The server
 * rendered real names; a first client render with an empty map produces
 * different DOM, Angular treats it as a hydration mismatch and rebuilds the
 * subtree, and the page shifts. Seeding the map synchronously at construction
 * is what lets the first client render agree with the server's.
 *
 * A Map does not survive `TransferState`'s JSON round trip, so entries go
 * across as an array of pairs.
 */
export function transferredMap<V>(name: string) {
  const state = inject(TransferState);
  const isBrowser = isPlatformBrowser(inject(PLATFORM_ID));
  const key = makeStateKey<[string, V][]>(name);

  return {
    /**
     * The map to start from: whatever the server left, or empty.
     *
     * Read once and removed. Leaving it would let a later navigation reuse a
     * snapshot from page load rather than fetching current data.
     */
    initial(): Map<string, V> {
      if (isBrowser && state.hasKey(key)) {
        const entries = state.get(key, [] as [string, V][]);
        state.remove(key);
        return new Map(entries);
      }
      return new Map();
    },

    /** Records the map for the browser. A no-op off the server. */
    publish(map: Map<string, V>): void {
      if (!isBrowser) {
        state.set(key, [...map.entries()]);
      }
    },
  };
}

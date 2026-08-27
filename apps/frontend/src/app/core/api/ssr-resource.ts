import {
  PLATFORM_ID,
  ResourceRef,
  TransferState,
  inject,
  makeStateKey,
  resource,
} from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

/**
 * A `resource()` whose first value survives the trip from the server render to
 * the browser.
 *
 * Angular 21 has no state transfer for resources. The HTTP responses behind
 * one *are* transferred — `provideClientHydration` handles that — but the
 * resource itself starts empty in the browser: `value()` undefined and
 * `isLoading()` true. So a page the server rendered with a full table renders
 * a loading skeleton on its first client pass, which is a hydration mismatch:
 * Angular discards the server's DOM, the page collapses to the height of the
 * skeleton, and grows back when the loader settles a tick later.
 *
 * That is two full-viewport layout shifts on every load of every
 * server-rendered page — measured at a cumulative 1.81 on the transfer feed,
 * where anything above 0.1 is considered poor. It also means the server render
 * is thrown away: all of SSR's cost, none of its benefit.
 *
 * Seeding the resource closes it. The server records each resolved value under
 * a key derived from the resource's name and its params; the browser reads it
 * back synchronously at construction and passes it as `defaultValue`, so the
 * first client render draws the same thing the server did and hydration finds
 * the DOM it expects.
 *
 * **The template has to cooperate**: branch on the value before `isLoading()`,
 * or the loading arm wins anyway and nothing is gained. See transfer-feed.ts.
 *
 * Only worth applying to routes in app.routes.server.ts that render on the
 * server. A client-only route has no server DOM to mismatch, so its skeleton
 * is the honest first paint.
 */
export function ssrResource<T, P>(
  name: string,
  options: {
    /**
     * As with `resource()`, returning `undefined` leaves the resource idle and
     * the loader uncalled — which is why the loader sees the narrowed type.
     */
    params?: () => P;
    loader: (args: { params: Exclude<P, undefined>; abortSignal: AbortSignal }) => Promise<T>;
  },
): ResourceRef<T | undefined> {
  const state = inject(TransferState);
  const isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  const keyFor = (params: P) => makeStateKey<T>(`${name}:${stableKey(params)}`);

  // Read synchronously, before the resource is created: `defaultValue` has to
  // be in hand for the very first render, which is the one that must match.
  let seeded: T | undefined;
  if (isBrowser) {
    const key = keyFor(options.params ? options.params() : (undefined as P));
    if (state.hasKey(key)) {
      seeded = state.get(key, undefined as T);
      // Taken, not borrowed. Leaving it would let a later navigation back to
      // these params render a value from page load rather than reloading.
      state.remove(key);
    }
  }

  return resource<T | undefined, P>({
    params: options.params,
    loader: async (args) => {
      const value = await options.loader(
        args as { params: Exclude<P, undefined>; abortSignal: AbortSignal },
      );
      if (!isBrowser) {
        state.set(keyFor(args.params as P), value);
      }
      return value;
    },
    defaultValue: seeded,
  });
}

/**
 * Key order has to match across the two renders, and object literals give no
 * such guarantee once a params object is built from more than one place, so
 * keys are sorted rather than trusted.
 */
function stableKey(params: unknown): string {
  return JSON.stringify(params, (_, value) => {
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      return Object.fromEntries(
        Object.entries(value as object).sort(([a], [b]) => a.localeCompare(b)),
      );
    }
    return value;
  });
}

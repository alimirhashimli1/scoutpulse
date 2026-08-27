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
 * Seeding the resource closes it. The server records its first resolved value;
 * the browser reads it back synchronously and passes it as `defaultValue`, so
 * the first client render draws what the server drew and hydration finds the
 * DOM it expects.
 *
 * **The template has to cooperate**: branch on the value before `isLoading()`,
 * or the loading arm wins anyway and nothing is gained.
 *
 * Only worth applying to routes that render on the server. A client-only route
 * has no server DOM to mismatch, so its skeleton is the honest first paint.
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

  /**
   * Keyed by name alone, deliberately — not by params.
   *
   * Keying by params would mean evaluating `params()` here, during field
   * initialization, and `resource()` does not: it treats params as a reactive
   * computation and reads it later. Calling it early reads fields declared
   * further down the class and required inputs before Angular has set them,
   * and both throw. The server never noticed, because it has nothing to seed
   * and skipped the call; the browser broke on every navigation.
   *
   * The name is enough. What is being carried is the value the server rendered
   * with, and there is exactly one of those per resource per render.
   */
  const key = makeStateKey<T>(name);

  // Read before the resource is created: `defaultValue` has to be in hand for
  // the very first render, which is the one that has to match.
  let seeded: T | undefined;
  if (isBrowser && state.hasKey(key)) {
    seeded = state.get(key, undefined as T);
    // Taken, not borrowed. Leaving it would let a later navigation back to
    // this route render page-load data rather than fetching current data.
    state.remove(key);
  }

  let published = false;

  return resource<T | undefined, P>({
    params: options.params,
    loader: async (args) => {
      const value = await options.loader(
        args as { params: Exclude<P, undefined>; abortSignal: AbortSignal },
      );
      // First write wins: it is the one whose output went into the HTML, and
      // so the one the browser's first render has to reproduce.
      if (!isBrowser && !published) {
        published = true;
        state.set(key, value);
      }
      return value;
    },
    defaultValue: seeded,
  });
}

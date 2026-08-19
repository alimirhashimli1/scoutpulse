import { InjectionToken } from '@angular/core';

/**
 * The public origin this app is served from — `https://scoutpulse.example`.
 *
 * Needed for canonical links, Open Graph URLs and the sitemap, all of which
 * must be **absolute**. A relative canonical is ignored, and a relative
 * `og:url` produces a link preview pointing at the crawler's own host.
 *
 * This is deliberately not `API_CONFIG`. That token answers "where do I fetch
 * data from", which inside Docker is an internal hostname no visitor can
 * reach. This one answers "what address is this page published at", and the
 * two are different strings in every deployment that has a domain name.
 */
export const SITE_URL = new InjectionToken<string>('SITE_URL');

/** Trailing slashes are stripped so joining a path never doubles them. */
export function normaliseOrigin(origin: string): string {
  return origin.replace(/\/+$/, '');
}

import { InjectionToken } from '@angular/core';

/**
 * Where the two backend services live.
 *
 * Both are reached through the gateway, which is the whole reason the gateway
 * exists: the frontend knows one origin rather than a port per service.
 */
export interface ApiConfig {
  /** Base for football-svc, e.g. http://localhost:8000/api/football */
  readonly football: string;
  /** Base for identity-svc, e.g. http://localhost:8000/api/identity */
  readonly identity: string;
}

/**
 * Injected rather than imported from a constants file, because the value
 * genuinely differs by platform.
 *
 * In the browser the gateway is reached at its public address. During server
 * rendering the Node process is *inside* the container network, where
 * `localhost` is the renderer itself and not the gateway at all — there it
 * must use the compose service name.
 *
 * Hardcoding the browser value would make SSR fail only once containerised,
 * which is a miserable thing to debug. A token makes the difference explicit
 * and lets tests provide their own.
 */
export const API_CONFIG = new InjectionToken<ApiConfig>('API_CONFIG');

/**
 * The gateway's address *from the browser*: the page's own origin.
 *
 * Empty on purpose. Every request becomes root-relative -- `/api/football`
 * rather than `https://somewhere/api/football` -- so the browser talks only to
 * the host it loaded the page from, whatever that host happens to be.
 *
 * This has to be empty rather than a real URL because the browser bundle is
 * built once and served everywhere: a literal address here is frozen at build
 * time and cannot be configured per deployment. It was `http://localhost:8000`,
 * which meant a deployed site asked *the visitor's own machine* for its data
 * and every request failed with ERR_CONNECTION_REFUSED.
 *
 * The requirement this places on a deployment is that something at the page's
 * origin routes `/api/*` onward -- Caddy does it locally and on Railway; a
 * `rewrites` entry does it on Vercel. That is the same-origin arrangement the
 * gateway existed for, and it is also what keeps CORS and cross-site cookies
 * out of the picture entirely.
 */
export const BROWSER_GATEWAY = '';

/**
 * Where the gateway is when nothing has said otherwise.
 *
 * Only for the Node renderer, which cannot use a relative URL -- it has no
 * origin to resolve one against, so `fetch` rejects it outright. Deployments
 * set `GATEWAY_INTERNAL_URL`; this is the local-development fallback.
 */
export const DEV_GATEWAY = 'http://localhost:8000';

export function apiConfigFor(gateway: string): ApiConfig {
  const base = gateway.replace(/\/+$/, '');
  return {
    football: `${base}/api/football`,
    identity: `${base}/api/identity`,
  };
}

/**
 * Points at each service directly, bypassing the gateway's path prefixes.
 *
 * There is a second way to run this stack that the gateway assumption above
 * silently excludes: `scripts\dev-run.ps1` starts the two services on their own
 * ports with no gateway at all, which is the documented path on a machine
 * without Docker — and the one `api.http` uses. Those services are reached at
 * `http://localhost:8081/api/v1/...`, with no `/api/football` in front, so
 * `apiConfigFor` produces a URL that 404s on every request.
 *
 * That it sets `CORS_ALLOWED_ORIGINS` to the frontend's origin is the giveaway:
 * the split mode was always meant to be usable from the app.
 */
export function apiConfigOf(football: string, identity: string): ApiConfig {
  return {
    football: football.replace(/\/+$/, ''),
    identity: identity.replace(/\/+$/, ''),
  };
}

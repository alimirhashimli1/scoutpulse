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

/** Default gateway address for a browser in local development. */
export const BROWSER_GATEWAY = 'http://localhost:8000';

export function apiConfigFor(gateway: string): ApiConfig {
  const base = gateway.replace(/\/+$/, '');
  return {
    football: `${base}/api/football`,
    identity: `${base}/api/identity`,
  };
}

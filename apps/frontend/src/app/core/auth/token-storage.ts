import { Injectable, PLATFORM_ID, inject } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

const REFRESH_TOKEN_KEY = 'scoutpulse.refresh_token';

/**
 * Where the refresh token lives between page loads.
 *
 * **The access token is deliberately not here.** It lives 15 minutes and is
 * held in memory by SessionStore, so it dies with the tab and cannot be read
 * out of storage by an injected script.
 *
 * The refresh token has to survive a reload — without it every refresh of the
 * page would sign the user out — so it goes in localStorage. That is a real
 * trade-off, not an oversight: localStorage is reachable by XSS. The
 * alternative, an HttpOnly cookie, would mean a second authentication
 * mechanism running alongside the bearer tokens every other endpoint uses, and
 * CSRF protection to go with it. Worth revisiting if this app ever renders
 * untrusted content.
 *
 * Every method is a no-op on the server. Reading storage during server
 * rendering throws, and the resulting error points nowhere near the cause —
 * this is the single most common way an SSR app breaks.
 */
@Injectable({ providedIn: 'root' })
export class TokenStorage {
  private readonly isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  read(): string | null {
    if (!this.isBrowser) return null;
    try {
      return localStorage.getItem(REFRESH_TOKEN_KEY);
    } catch {
      // Storage can throw in private browsing modes and when a policy blocks
      // it. Signed-out is a working state; a crash is not.
      return null;
    }
  }

  write(token: string): void {
    if (!this.isBrowser) return;
    try {
      localStorage.setItem(REFRESH_TOKEN_KEY, token);
    } catch {
      // Ignored: the session still works for this tab, it just will not
      // survive a reload.
    }
  }

  clear(): void {
    if (!this.isBrowser) return;
    try {
      localStorage.removeItem(REFRESH_TOKEN_KEY);
    } catch {
      // Nothing useful to do — the caller is signing out either way.
    }
  }
}

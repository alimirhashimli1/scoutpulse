import { Injectable, PLATFORM_ID, inject } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

const SIGNED_IN_KEY = 'scoutpulse.signed_in';
/** Where the refresh token itself was kept before it moved to a cookie. */
const LEGACY_REFRESH_TOKEN_KEY = 'scoutpulse.refresh_token';

/**
 * What this browser remembers about a session between page loads.
 *
 * **No credential is stored here.** The refresh token lives in an HttpOnly
 * cookie the identity service sets, which no script -- ours or an injected
 * one -- can read; the access token lives in memory (SessionStore) and dies
 * with the tab. What is kept is a flag saying a session cookie probably
 * exists, so a page load knows whether restoring is worth a request: the
 * cookie cannot be seen from here, and asking on every visit would spend a
 * refresh call on every anonymous visitor.
 *
 * A token left in storage by the earlier, localStorage-based version is read
 * once, presented to move it onto the cookie, and deleted.
 *
 * Every method is a no-op on the server. Reading storage during server
 * rendering throws, and the resulting error points nowhere near the cause —
 * this is the single most common way an SSR app breaks.
 */
@Injectable({ providedIn: 'root' })
export class TokenStorage {
  private readonly isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  /** Whether a session may be waiting to be restored. */
  hasSession(): boolean {
    return this.get(SIGNED_IN_KEY) === '1' || this.legacyToken() !== null;
  }

  /** A refresh token from before the cookie, if one is still stored. */
  legacyToken(): string | null {
    return this.get(LEGACY_REFRESH_TOKEN_KEY);
  }

  /** Records that the cookie now holds the session; drops any legacy token. */
  markSignedIn(): void {
    this.set(SIGNED_IN_KEY, '1');
    this.remove(LEGACY_REFRESH_TOKEN_KEY);
  }

  clear(): void {
    this.remove(SIGNED_IN_KEY);
    this.remove(LEGACY_REFRESH_TOKEN_KEY);
  }

  // Storage can throw in private browsing modes and when a policy blocks it.
  // Signed-out is a working state; a crash is not.
  private get(key: string): string | null {
    if (!this.isBrowser) return null;
    try {
      return localStorage.getItem(key);
    } catch {
      return null;
    }
  }

  private set(key: string, value: string): void {
    if (!this.isBrowser) return;
    try {
      localStorage.setItem(key, value);
    } catch {
      // The session still works for this tab, it just will not be restored
      // on a reload.
    }
  }

  private remove(key: string): void {
    if (!this.isBrowser) return;
    try {
      localStorage.removeItem(key);
    } catch {
      // Nothing useful to do.
    }
  }
}

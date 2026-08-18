import { Injectable, computed, inject, signal } from '@angular/core';

import { AccessTokenClaims, Role, TokenPair, User } from '../models/identity';
import { TokenStorage } from './token-storage';

export type SessionStatus = 'unknown' | 'anonymous' | 'authenticated';

/**
 * The single source of truth for who is signed in.
 *
 * Everything derives from three signals rather than being stored separately —
 * `isAdmin` is computed from the role, not a field that could disagree with it.
 * That is the whole point: there is no state to keep in step.
 *
 * The access token is held **in memory only**. See TokenStorage for why.
 */
@Injectable({ providedIn: 'root' })
export class SessionStore {
  private readonly storage = inject(TokenStorage);

  private readonly _accessToken = signal<string | null>(null);
  private readonly _user = signal<User | null>(null);
  private readonly _status = signal<SessionStatus>('unknown');

  readonly accessToken = this._accessToken.asReadonly();
  readonly user = this._user.asReadonly();

  /**
   * `unknown` until a restore attempt has finished.
   *
   * Guards and the header must distinguish "not signed in" from "we have not
   * looked yet", or a reload flashes the signed-out UI before the session
   * comes back.
   */
  readonly status = this._status.asReadonly();

  readonly isAuthenticated = computed(() => this._status() === 'authenticated');
  readonly role = computed<Role | null>(() => this._user()?.role ?? null);
  readonly isAdmin = computed(() => this.role() === 'admin');
  /** Admins can do everything an editor can, so this includes them. */
  readonly isEditor = computed(() => this.role() === 'admin' || this.role() === 'editor');

  readonly refreshToken = (): string | null => this.storage.read();

  /** Records a completed sign-in, from login, refresh or the OAuth exchange. */
  start(tokens: TokenPair, user: User | null = null): void {
    this._accessToken.set(tokens.access_token);
    this.storage.write(tokens.refresh_token);
    if (user) this._user.set(user);
    this._status.set('authenticated');
  }

  /**
   * Updates the tokens without touching the user.
   *
   * Used by the refresh interceptor, which has new tokens but no fresh profile
   * and must not blank the one already loaded.
   */
  renew(tokens: TokenPair): void {
    this._accessToken.set(tokens.access_token);
    this.storage.write(tokens.refresh_token);
    this._status.set('authenticated');
  }

  setUser(user: User): void {
    this._user.set(user);
  }

  /** Ends the session locally. Revoking server-side is the caller's job. */
  clear(): void {
    this._accessToken.set(null);
    this._user.set(null);
    this.storage.clear();
    this._status.set('anonymous');
  }

  /** Marks the restore attempt finished with nobody signed in. */
  markAnonymous(): void {
    this._status.set('anonymous');
  }

  /**
   * Reads the claims out of the current access token.
   *
   * For display and for hiding controls only. Never a security boundary — the
   * API re-checks every rule, and this is client-side data the user could
   * rewrite. It is also why a role change needs a fresh login to show up:
   * the role here is whatever was true when the token was minted.
   */
  claims(): AccessTokenClaims | null {
    const token = this._accessToken();
    if (!token) return null;

    const payload = token.split('.')[1];
    if (!payload) return null;

    try {
      const json = atob(payload.replace(/-/g, '+').replace(/_/g, '/'));
      return JSON.parse(json) as AccessTokenClaims;
    } catch {
      return null;
    }
  }
}

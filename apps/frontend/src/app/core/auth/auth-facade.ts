import { Injectable, inject } from '@angular/core';

import { ApiError } from '../api/api-error';
import { AuthRepository, LoginRequest, RegisterRequest } from './auth-repository';
import { SessionStore } from './session-store';

/**
 * Orchestrates sign-in, so components never sequence these calls themselves.
 *
 * Logging in is two requests — the token pair, then the profile — and both have
 * to land before the session is usable. Putting that in a component would mean
 * repeating it in the login page, the register page and the OAuth callback,
 * and getting it subtly different in each.
 */
@Injectable({ providedIn: 'root' })
export class AuthFacade {
  private readonly auth = inject(AuthRepository);
  private readonly session = inject(SessionStore);

  async login(credentials: LoginRequest): Promise<void> {
    const tokens = await this.auth.login(credentials);
    this.session.start(tokens);
    await this.loadProfile();
  }

  /**
   * Registers, then signs in.
   *
   * Registration returns the new user but no tokens — the API deliberately
   * does not sign you in as a side effect of creating an account. Doing it
   * here keeps the two-step nature out of the page.
   */
  /**
   * Creates the account, and signs in only when it is usable immediately.
   *
   * With verification enabled the new account cannot log in yet — the API
   * refuses it until the address is confirmed — so attempting it here would
   * turn a successful registration into an error message.
   *
   * Returns whether a confirmation step is outstanding, so the page can say so.
   */
  async register(details: RegisterRequest): Promise<boolean> {
    const result = await this.auth.register(details);
    if (result.verification_required) return true;

    await this.login({ identifier: details.username, password: details.password });
    return false;
  }

  /** Completes a provider sign-in from the one-time code in the callback URL. */
  async completeOAuth(code: string): Promise<void> {
    const tokens = await this.auth.exchange(code);
    this.session.start(tokens);
    await this.loadProfile();
  }

  /**
   * Ends the session.
   *
   * The local session is cleared whatever the server says. A logout that fails
   * because the token was already revoked has still achieved what the user
   * asked for, and leaving them signed in to argue the point would be absurd.
   */
  async logout(): Promise<void> {
    const refreshToken = this.session.refreshToken();
    try {
      if (refreshToken) await this.auth.logout(refreshToken);
    } catch {
      // Deliberately ignored — see above.
    } finally {
      this.session.clear();
    }
  }

  /**
   * Restores a session on page load, in the browser only.
   *
   * The access token lives in memory and is gone after a reload, so the stored
   * refresh token is exchanged for a new pair. Without this every refresh of
   * the page would sign the user out.
   *
   * A failure here is normal, not exceptional: the token expires after 30 days,
   * and the user may have signed out in another tab. It resolves to anonymous
   * rather than throwing, because "not signed in" is a working state.
   */
  async restore(): Promise<void> {
    const refreshToken = this.session.refreshToken();
    if (!refreshToken) {
      this.session.markAnonymous();
      return;
    }

    try {
      const tokens = await this.auth.refresh(refreshToken);
      this.session.renew(tokens);
      await this.loadProfile();
    } catch {
      this.session.clear();
    }
  }

  private async loadProfile(): Promise<void> {
    try {
      this.session.setUser(await this.auth.me());
    } catch (error) {
      // The tokens are valid but the profile did not load — a transient
      // failure, most likely. Keep the session rather than signing the user
      // out over it; the profile can be fetched again.
      if (error instanceof ApiError && error.code === 'unauthorized') {
        this.session.clear();
      }
    }
  }
}

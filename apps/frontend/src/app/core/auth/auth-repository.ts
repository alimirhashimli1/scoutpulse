import { HttpClient, HttpContext, HttpContextToken } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';

import { API_CONFIG } from '../tokens/api-config';
import { Identity, TokenPair, User } from '../models/identity';

/**
 * Marks a request as part of the authentication flow itself.
 *
 * The refresh interceptor checks this and leaves such requests alone. Without
 * it, a failing refresh would trigger a refresh, which would fail, which would
 * trigger a refresh — an infinite loop that presents as the tab hanging.
 */
export const SKIP_AUTH = new HttpContextToken<boolean>(() => false);

const skipAuth = () => ({ context: new HttpContext().set(SKIP_AUTH, true) });

export interface RegisterRequest {
  username: string;
  email: string;
  password: string;
}

export interface LoginRequest {
  /** Username or email — the API accepts either. */
  identifier: string;
  password: string;
}

/**
 * Everything identity-svc exposes.
 *
 * Not split into reader and writer like the football repositories: there is no
 * meaningful read-only consumer of authentication, and a page that can call
 * `login` can necessarily call `logout`.
 */
@Injectable({ providedIn: 'root' })
export class AuthRepository {
  private readonly http = inject(HttpClient);
  private readonly api = inject(API_CONFIG);

  private url(path: string): string {
    return `${this.api.identity}/api/v1/${path}`;
  }

  register(body: RegisterRequest): Promise<User> {
    return firstValueFrom(this.http.post<User>(this.url('auth/register'), body, skipAuth()));
  }

  login(body: LoginRequest): Promise<TokenPair> {
    return firstValueFrom(this.http.post<TokenPair>(this.url('auth/login'), body, skipAuth()));
  }

  /**
   * Exchanges a refresh token for a new pair.
   *
   * The old token is destroyed server-side by this call. Presenting it again
   * is treated as a leaked credential and revokes **every** session the user
   * holds — which is why exactly one of these may ever be in flight. See
   * refreshInterceptor.
   */
  refresh(refreshToken: string): Promise<TokenPair> {
    return firstValueFrom(
      this.http.post<TokenPair>(this.url('auth/refresh'), { refresh_token: refreshToken }, skipAuth()),
    );
  }

  logout(refreshToken: string): Promise<void> {
    return firstValueFrom(
      this.http.post<void>(this.url('auth/logout'), { refresh_token: refreshToken }, skipAuth()),
    );
  }

  me(): Promise<User> {
    return firstValueFrom(this.http.get<User>(this.url('users/me')));
  }

  changePassword(currentPassword: string, newPassword: string): Promise<void> {
    return firstValueFrom(
      this.http.put<void>(this.url('users/me/password'), {
        current_password: currentPassword,
        new_password: newPassword,
      }),
    );
  }

  // --- external sign-in -----------------------------------------------

  /** Which providers this deployment has configured. */
  async providers(): Promise<string[]> {
    const body = await firstValueFrom(
      this.http.get<{ providers: string[] | null }>(this.url('auth/providers'), skipAuth()),
    );
    return body.providers ?? [];
  }

  /**
   * Where to send the browser to begin a provider sign-in.
   *
   * A full-page navigation, not an XHR: the provider will redirect, and a
   * fetch cannot follow that to a consent screen the user has to see.
   */
  providerStartUrl(provider: string): string {
    return this.url(`auth/${provider}`);
  }

  /**
   * Trades the one-time code from the callback for a token pair.
   *
   * The code is single-use and expires in 60 seconds, which is why tokens are
   * not put in the redirect URL — they would land in browser history, access
   * logs and the next page's Referer header.
   */
  exchange(code: string): Promise<TokenPair> {
    return firstValueFrom(
      this.http.post<TokenPair>(this.url('auth/exchange'), { code }, skipAuth()),
    );
  }

  async identities(): Promise<Identity[]> {
    const body = await firstValueFrom(
      this.http.get<{ identities: Identity[] | null }>(this.url('users/me/identities')),
    );
    return body.identities ?? [];
  }

  unlinkIdentity(provider: string): Promise<void> {
    return firstValueFrom(this.http.delete<void>(this.url(`users/me/identities/${provider}`)));
  }
}

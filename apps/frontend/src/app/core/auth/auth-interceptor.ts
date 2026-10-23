import { HttpInterceptorFn, HttpRequest } from '@angular/common/http';
import { inject } from '@angular/core';
import { Observable, catchError, from, switchMap, throwError } from 'rxjs';

import { TokenPair } from '../models/identity';
import { AuthRepository, SKIP_AUTH, isUnauthorized } from './auth-repository';
import { SessionStore } from './session-store';

/**
 * A refresh that is already running.
 *
 * Module scope on purpose. Every request passes through a *new* invocation of
 * the interceptor function, so anything held inside it is gone by the time the
 * next request arrives — there would be nothing to share, and each 401 would
 * start its own refresh.
 *
 * That is precisely the bug this exists to prevent, and it is worth being
 * concrete about the consequence:
 *
 *   A refresh token is single use. Presenting one that has already been
 *   exchanged is treated by the backend as a leaked credential, and it revokes
 *   EVERY session the user holds.
 *
 * An access token expiring mid-page routinely produces three or four
 * simultaneous 401s. Refresh each one and the first succeeds, the rest present
 * a spent token, and the user is signed out everywhere — for doing nothing but
 * loading a page. The symptom is random logouts that are very hard to trace.
 *
 * So: the first 401 starts a refresh, every other request waits on that same
 * promise, and all of them retry with whatever it produced.
 */
let inFlightRefresh: Promise<TokenPair> | null = null;

/** Exposed for tests, which need a clean slate between cases. */
export function resetRefreshState(): void {
  inFlightRefresh = null;
}

export const authInterceptor: HttpInterceptorFn = (req, next) => {
  const session = inject(SessionStore);
  const auth = inject(AuthRepository);

  // Login, register, refresh, logout and the code exchange must never be
  // retried by this interceptor. A failing refresh triggering a refresh is an
  // infinite loop.
  if (req.context.get(SKIP_AUTH)) {
    return next(req);
  }

  const send = (token: string | null) => next(token ? withBearer(req, token) : req);

  return send(session.accessToken()).pipe(
    catchError((error: unknown) => {
      if (!isUnauthorized(error)) {
        return throwError(() => error);
      }

      // No session to refresh — the user is simply signed out.
      if (!session.hasStoredSession()) {
        session.clear();
        return throwError(() => error);
      }

      return from(refreshOnce(auth, session)).pipe(
        switchMap((tokens) => send(tokens.access_token)),
        catchError((refreshError: unknown) => {
          // Refused: the session expired, was revoked, or was signed out in
          // another tab. There is no way back without signing in again. Any
          // other failure leaves the session for the next attempt.
          if (isUnauthorized(refreshError)) session.clear();
          return throwError(() => refreshError);
        }),
      );
    }),
  );
};

/**
 * Runs at most one refresh at a time.
 *
 * Callers arriving while one is in flight get the same promise rather than
 * starting another. The slot is cleared in `finally` so a *later* 401 can
 * refresh again — leaving it set would permanently reuse a stale result.
 */
function refreshOnce(auth: AuthRepository, session: SessionStore): Promise<TokenPair> {
  inFlightRefresh ??= auth
    .refreshSession(session.legacyRefreshToken())
    .then((tokens) => {
      session.renew(tokens);
      return tokens;
    })
    .finally(() => {
      inFlightRefresh = null;
    });

  return inFlightRefresh;
}

function withBearer(req: HttpRequest<unknown>, token: string): HttpRequest<unknown> {
  return req.clone({ setHeaders: { Authorization: `Bearer ${token}` } });
}

/** Re-exported so the config can read it without importing rxjs. */
export type { Observable };

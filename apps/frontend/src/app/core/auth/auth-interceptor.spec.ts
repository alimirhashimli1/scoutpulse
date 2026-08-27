import {
  HttpClient,
  HttpContext,
  HttpErrorResponse,
  provideHttpClient,
  withInterceptors,
} from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';

import { API_CONFIG, apiConfigFor } from '../tokens/api-config';
import { TokenPair } from '../models/identity';
import { SKIP_AUTH } from './auth-repository';
import { authInterceptor, resetRefreshState } from './auth-interceptor';
import { SessionStore } from './session-store';

const GATEWAY = 'http://test.local';
const api = apiConfigFor(GATEWAY);
const REFRESH_URL = `${api.identity}/api/v1/auth/refresh`;
const PROTECTED_URL = `${api.football}/api/v1/players`;

/**
 * Lets every pending microtask and promise callback run.
 *
 * The interceptor bridges a promise (the refresh) back into an observable
 * chain, so the retries are queued several microtask turns after the refresh
 * resolves. Awaiting a fixed number of Promise.resolve() calls is guessing at
 * that depth; a macrotask drains the whole queue whatever it is.
 */
const flush = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

function tokenPair(suffix: string): TokenPair {
  return {
    access_token: `access-${suffix}`,
    refresh_token: `refresh-${suffix}`,
    token_type: 'Bearer',
    expires_in: 900,
  };
}

describe('authInterceptor', () => {
  let http: HttpClient;
  let controller: HttpTestingController;
  let session: SessionStore;

  beforeEach(() => {
    // Module-scoped, so it survives between tests and would otherwise leak.
    resetRefreshState();
    localStorage.clear();

    TestBed.configureTestingModule({
      providers: [
        provideHttpClient(withInterceptors([authInterceptor])),
        provideHttpClientTesting(),
        { provide: API_CONFIG, useValue: api },
      ],
    });

    http = TestBed.inject(HttpClient);
    controller = TestBed.inject(HttpTestingController);
    session = TestBed.inject(SessionStore);

    session.start(tokenPair('original'));
  });

  afterEach(() => {
    controller.verify();
    localStorage.clear();
  });

  it('attaches the access token', () => {
    http.get(PROTECTED_URL).subscribe();

    const req = controller.expectOne(PROTECTED_URL);
    expect(req.request.headers.get('Authorization')).toBe('Bearer access-original');
    req.flush({ items: [] });
  });

  /**
   * The test this interceptor exists for.
   *
   * A refresh token is single use, and presenting a spent one makes the backend
   * revoke every session the user holds. An expiring access token routinely
   * produces several simultaneous 401s, so if each one refreshed independently
   * the user would be signed out of everything for merely loading a page.
   */
  it('issues ONE refresh for three simultaneous 401s', async () => {
    const results: unknown[] = [];
    http.get(`${PROTECTED_URL}/1`).subscribe((r) => results.push(r));
    http.get(`${PROTECTED_URL}/2`).subscribe((r) => results.push(r));
    http.get(`${PROTECTED_URL}/3`).subscribe((r) => results.push(r));

    // All three arrive before any response, as they do in a real page load.
    const initial = controller.match((r) => r.url.startsWith(PROTECTED_URL));
    expect(initial.length).toBe(3);
    initial.forEach((r) => r.flush(null, { status: 401, statusText: 'Unauthorized' }));

    // Exactly one refresh — this is the assertion that matters.
    const refreshes = controller.match(REFRESH_URL);
    expect(refreshes.length).toBe(1);
    refreshes[0].flush(tokenPair('renewed'));

    await flush();

    // And all three retry with the new token.
    const retried = controller.match((r) => r.url.startsWith(PROTECTED_URL));
    expect(retried.length).toBe(3);
    retried.forEach((r) => {
      expect(r.request.headers.get('Authorization')).toBe('Bearer access-renewed');
      r.flush({ ok: true });
    });

    expect(results.length).toBe(3);
  });

  it('sends the stored refresh token, not the access token', async () => {
    http.get(PROTECTED_URL).subscribe({ error: () => undefined });

    controller.expectOne(PROTECTED_URL).flush(null, { status: 401, statusText: 'Unauthorized' });

    const refresh = controller.expectOne(REFRESH_URL);
    expect(refresh.request.body).toEqual({ refresh_token: 'refresh-original' });
    refresh.flush(tokenPair('renewed'));

    await flush();
    controller.expectOne(PROTECTED_URL).flush({ ok: true });
  });

  it('stores the rotated tokens, since the old pair is now dead', async () => {
    http.get(PROTECTED_URL).subscribe({ error: () => undefined });
    controller.expectOne(PROTECTED_URL).flush(null, { status: 401, statusText: 'Unauthorized' });
    controller.expectOne(REFRESH_URL).flush(tokenPair('renewed'));

    await flush();

    expect(session.accessToken()).toBe('access-renewed');
    expect(session.refreshToken()).toBe('refresh-renewed');

    controller.expectOne(PROTECTED_URL).flush({ ok: true });
  });

  it('allows a later 401 to refresh again', async () => {
    // The in-flight slot must be released, or the second refresh would reuse
    // a stale result forever.
    http.get(PROTECTED_URL).subscribe({ error: () => undefined });
    controller.expectOne(PROTECTED_URL).flush(null, { status: 401, statusText: 'Unauthorized' });
    controller.expectOne(REFRESH_URL).flush(tokenPair('first'));
    await flush();
    controller.expectOne(PROTECTED_URL).flush({ ok: true });

    http.get(PROTECTED_URL).subscribe({ error: () => undefined });
    controller.expectOne(PROTECTED_URL).flush(null, { status: 401, statusText: 'Unauthorized' });
    const second = controller.expectOne(REFRESH_URL);
    expect(second.request.body).toEqual({ refresh_token: 'refresh-first' });
    second.flush(tokenPair('second'));
    await flush();
    controller.expectOne(PROTECTED_URL).flush({ ok: true });
  });

  it('clears the session when the refresh itself fails', async () => {
    http.get(PROTECTED_URL).subscribe({ error: () => undefined });
    controller.expectOne(PROTECTED_URL).flush(null, { status: 401, statusText: 'Unauthorized' });

    // What the backend returns when the token was already spent or revoked.
    controller.expectOne(REFRESH_URL).flush(null, { status: 401, statusText: 'Unauthorized' });
    await flush();

    expect(session.isAuthenticated()).toBe(false);
    expect(session.refreshToken()).toBeNull();
  });

  it('does not attempt a refresh with no refresh token', () => {
    session.clear();

    http.get(PROTECTED_URL).subscribe({ error: () => undefined });
    controller.expectOne(PROTECTED_URL).flush(null, { status: 401, statusText: 'Unauthorized' });

    // No refresh request at all — afterEach's verify() would fail on a stray one.
    expect(session.isAuthenticated()).toBe(false);
  });

  it('never refreshes a failing refresh, which would loop forever', () => {
    // SKIP_AUTH is what AuthRepository puts on every auth call. Without it the
    // interceptor would see this 401, try to refresh, get another 401, and
    // recurse — which is the loop the flag exists to prevent.
    http
      .post(
        REFRESH_URL,
        { refresh_token: 'x' },
        { context: new HttpContext().set(SKIP_AUTH, true) },
      )
      .subscribe({ error: () => undefined });

    controller.expectOne(REFRESH_URL).flush(null, { status: 401, statusText: 'Unauthorized' });

    // afterEach's verify() fails if a second refresh was issued.
  });

  it('passes non-401 failures through untouched', () => {
    let captured: unknown;
    http.get(PROTECTED_URL).subscribe({ error: (e) => (captured = e) });

    controller
      .expectOne(PROTECTED_URL)
      .flush({ error: 'nope', code: 'forbidden' }, { status: 403, statusText: 'Forbidden' });

    expect(captured).toBeTruthy();
    expect(session.isAuthenticated()).toBe(true);
  });

  it('does not retry a request that had no token to begin with', () => {
    session.clear();

    let captured: unknown;
    http.get(PROTECTED_URL).subscribe({ error: (e) => (captured = e) });

    const req = controller.expectOne(PROTECTED_URL);
    expect(req.request.headers.has('Authorization')).toBe(false);
    req.flush(null, { status: 401, statusText: 'Unauthorized' });

    expect((captured as HttpErrorResponse).status).toBe(401);
  });
});

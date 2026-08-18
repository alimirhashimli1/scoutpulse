import { inject } from '@angular/core';
import { CanActivateFn, Router, UrlTree } from '@angular/router';

import { SessionStore } from './session-store';

/**
 * Route guards.
 *
 * **These are not security.** The API re-checks every rule independently, and
 * anything a guard consults — the role in a token, a signal in memory — is
 * client-side data the user could change. A guard exists so somebody is not
 * shown a page that would immediately 403 at them.
 *
 * Every route these protect is `RenderMode.Client`, so they never run during
 * server rendering, where there is no session to consult.
 */

function redirectToLogin(router: Router, attemptedUrl: string): UrlTree {
  // Carry where they were going, so signing in returns them there rather than
  // dumping them on the home page.
  return router.createUrlTree(['/login'], { queryParams: { returnTo: attemptedUrl } });
}

export const authGuard: CanActivateFn = (_route, state) => {
  const session = inject(SessionStore);
  const router = inject(Router);

  return session.isAuthenticated() ? true : redirectToLogin(router, state.url);
};

export const adminGuard: CanActivateFn = (_route, state) => {
  const session = inject(SessionStore);
  const router = inject(Router);

  if (!session.isAuthenticated()) return redirectToLogin(router, state.url);

  // Signed in but not permitted: home, not the login page. Sending them to
  // sign in again would imply the wrong credentials fix it.
  return session.isAdmin() ? true : router.createUrlTree(['/']);
};

export const editorGuard: CanActivateFn = (_route, state) => {
  const session = inject(SessionStore);
  const router = inject(Router);

  if (!session.isAuthenticated()) return redirectToLogin(router, state.url);
  return session.isEditor() ? true : router.createUrlTree(['/']);
};

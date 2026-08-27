import {
  ApplicationConfig,
  PLATFORM_ID,
  inject,
  provideAppInitializer,
  provideBrowserGlobalErrorListeners,
  provideZonelessChangeDetection,
} from '@angular/core';
import { DOCUMENT, isPlatformBrowser } from '@angular/common';
import { provideRouter, withComponentInputBinding, withInMemoryScrolling } from '@angular/router';
import { provideHttpClient, withFetch, withInterceptors } from '@angular/common/http';
import { provideClientHydration, withEventReplay } from '@angular/platform-browser';

import { routes } from './app.routes';
import { API_CONFIG, apiConfigFor, BROWSER_GATEWAY } from './core/tokens/api-config';
import { SITE_URL, normaliseOrigin } from './core/tokens/site-url';
import { correlationIdInterceptor, errorInterceptor } from './core/api/interceptors';
import { provideFootballApi } from './core/api/providers';
import { authInterceptor } from './core/auth/auth-interceptor';
import { AuthFacade } from './core/auth/auth-facade';
import { SessionStore } from './core/auth/session-store';

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideZonelessChangeDetection(),

    provideRouter(
      routes,
      // Route params arrive as component inputs, so a detail component does not
      // need to inject ActivatedRoute just to read an id.
      withComponentInputBinding(),
      // Navigating to a new page starts at the top; a fragment scrolls to it.
      withInMemoryScrolling({ scrollPositionRestoration: 'enabled', anchorScrolling: 'enabled' }),
    ),

    // withFetch matters for SSR: the fetch backend lets Angular transfer
    // responses from the server render to the client, so a page rendered on the
    // server does not immediately re-request everything on hydration.
    //
    // Interceptor order is the execution order. The correlation id is attached
    // on the way out; the error translation wraps everything inside it, so it
    // sees failures from every later interceptor too — including the auth
    // refresh that arrives in phase 2.
    // Interceptor order is execution order, outermost first.
    //
    // errorInterceptor is outermost so it sees failures from everything inside
    // it — including a refresh that fails. authInterceptor sits inside it and
    // attaches the bearer token, retrying once behind a single-flight refresh.
    provideHttpClient(
      withFetch(),
      withInterceptors([errorInterceptor, authInterceptor, correlationIdInterceptor]),
    ),

    provideClientHydration(withEventReplay()),

    // The browser talks to the gateway at its public address. The server
    // override lives in app.config.server.ts — see docs/FRONTEND.md §11.3.
    { provide: API_CONFIG, useValue: apiConfigFor(BROWSER_GATEWAY) },

    // The address this page is published at, for canonical and og:url. In the
    // browser it is simply where we are; the server cannot know without being
    // told, so it reads SITE_URL from the environment.
    {
      provide: SITE_URL,
      useFactory: () => {
        const document = inject(DOCUMENT);
        return normaliseOrigin(document.location?.origin ?? 'http://localhost:4000');
      },
    },

    // Every repository contract bound to its HTTP implementation.
    ...provideFootballApi(),

    // Restore the session before the first route renders.
    //
    // The access token lives in memory and is gone after a reload, so the
    // stored refresh token is exchanged for a new pair. Browser only: the
    // server has no storage to read, and blocking server rendering on a
    // network call for a session it cannot have would slow every page for
    // nothing.
    provideAppInitializer(() => {
      const session = inject(SessionStore);
      if (!isPlatformBrowser(inject(PLATFORM_ID))) {
        session.markAnonymous();
        return;
      }
      return inject(AuthFacade).restore();
    }),
  ],
};

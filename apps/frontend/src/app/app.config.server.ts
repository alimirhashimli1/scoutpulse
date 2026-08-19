import { mergeApplicationConfig, ApplicationConfig } from '@angular/core';
import { provideServerRendering, withRoutes } from '@angular/ssr';

import { appConfig } from './app.config';
import { serverRoutes } from './app.routes.server';
import { API_CONFIG, apiConfigFor, BROWSER_GATEWAY } from './core/tokens/api-config';
import { SITE_URL, normaliseOrigin } from './core/tokens/site-url';

/**
 * Where the gateway is *from the Node renderer*.
 *
 * This is a different network position from the browser's. Running in Docker,
 * `localhost` inside this process is the renderer itself, not the gateway —
 * so the compose service name is used instead. Outside Docker the browser
 * address is correct, which is why it is the fallback.
 *
 * Getting this wrong makes SSR fail only once containerised, which is a
 * miserable thing to debug: the app works in `ng serve` and returns empty
 * pages in production.
 */
const serverGateway = process.env['GATEWAY_INTERNAL_URL'] ?? BROWSER_GATEWAY;

/**
 * The public address, for canonical links and Open Graph URLs.
 *
 * The browser can read its own origin; the renderer cannot — the `Host` header
 * is attacker-controlled, and building canonical links from it would let
 * anyone point this site's canonical tags at theirs. So it is configuration,
 * and **must be set to the real domain in production**. The default only suits
 * local use.
 */
const siteUrl = normaliseOrigin(process.env['SITE_URL'] ?? 'http://localhost:4000');

const serverConfig: ApplicationConfig = {
  providers: [
    provideServerRendering(withRoutes(serverRoutes)),
    { provide: API_CONFIG, useValue: apiConfigFor(serverGateway) },
    { provide: SITE_URL, useValue: siteUrl },
  ],
};

export const config = mergeApplicationConfig(appConfig, serverConfig);

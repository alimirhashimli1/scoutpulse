import { mergeApplicationConfig, ApplicationConfig } from '@angular/core';
import { provideServerRendering, withRoutes } from '@angular/ssr';

import { appConfig } from './app.config';
import { serverRoutes } from './app.routes.server';
import { API_CONFIG, apiConfigFor, BROWSER_GATEWAY } from './core/tokens/api-config';

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

const serverConfig: ApplicationConfig = {
  providers: [
    provideServerRendering(withRoutes(serverRoutes)),
    { provide: API_CONFIG, useValue: apiConfigFor(serverGateway) },
  ],
};

export const config = mergeApplicationConfig(appConfig, serverConfig);

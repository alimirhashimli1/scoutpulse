import {
  AngularNodeAppEngine,
  createNodeRequestHandler,
  isMainModule,
  writeResponseToNodeResponse,
} from '@angular/ssr/node';
import express from 'express';
import { join } from 'node:path';

const browserDistFolder = join(import.meta.dirname, '../browser');

/**
 * Hosts this renderer will answer for.
 *
 * Angular 21 validates the `Host` header against this list. It is not
 * bureaucracy: the header is attacker-controlled, and the renderer builds
 * absolute URLs from it. An unchecked host lets an attacker point server-side
 * fetches at an address of their choosing — server-side request forgery — and
 * poison any cache keyed on the rendered output.
 *
 * With no list configured, Angular logs and silently falls back to
 * client-side rendering, which looks like "SSR mysteriously does nothing".
 * That is exactly what happened the first time this was run.
 *
 * Set NG_ALLOWED_HOSTS to the real domain in production. The defaults below
 * only cover local development and the compose service name.
 */
const allowedHosts = (process.env['NG_ALLOWED_HOSTS'] ?? 'localhost,127.0.0.1,frontend')
  .split(',')
  .map((host) => host.trim())
  .filter(Boolean);

/**
 * Whether to believe the `X-Forwarded-*` headers in front of this process.
 *
 * This renderer never faces the internet directly — Caddy sits in front of it
 * locally and on Railway, Vercel's edge does on Vercel. In every one of those
 * positions the `Host` header carries the proxy's own address and the real one
 * arrives in `X-Forwarded-Host`. Angular ignores those headers unless told
 * otherwise, so it checks the proxy's hostname against `allowedHosts` above,
 * fails to find it, and falls back to client-side rendering — quietly, with a
 * warning about this option and a `200` carrying an empty shell. The routes in
 * app.routes.server.ts stop applying too, so a mistyped URL answers `200`
 * instead of the `404` declared for it.
 *
 * Off by default, and deliberately so: trusting these headers is only sound
 * because something upstream sets them. Angular's own guidance is that they be
 * validated at the proxy, which is exactly what makes it safe here and unsafe
 * for a process someone exposes directly. A deployment that puts a proxy in
 * front sets TRUST_PROXY_HEADERS; one that does not, must not.
 */
const trustProxyHeaders = process.env['TRUST_PROXY_HEADERS'] === 'true';

const app = express();
const angularApp = new AngularNodeAppEngine({ allowedHosts, trustProxyHeaders });

/**
 * Example Express Rest API endpoints can be defined here.
 * Uncomment and define endpoints as necessary.
 *
 * Example:
 * ```ts
 * app.get('/api/{*splat}', (req, res) => {
 *   // Handle API request
 * });
 * ```
 */

/**
 * Serve static files from /browser
 */
app.use(
  express.static(browserDistFolder, {
    maxAge: '1y',
    index: false,
    redirect: false,
  }),
);

/**
 * Handle all other requests by rendering the Angular application.
 */
app.use((req, res, next) => {
  angularApp
    .handle(req)
    .then((response) => (response ? writeResponseToNodeResponse(response, res) : next()))
    .catch(next);
});

/**
 * Start the server if this module is the main entry point, or it is ran via PM2.
 * The server listens on the port defined by the `PORT` environment variable, or defaults to 4000.
 */
if (isMainModule(import.meta.url) || process.env['pm_id']) {
  const port = process.env['PORT'] || 4000;
  app.listen(port, (error) => {
    if (error) {
      throw error;
    }

    console.log(`Node Express server listening on http://localhost:${port}`);
  });
}

/**
 * Request handler used by the Angular CLI (for dev-server and during build) or Firebase Cloud Functions.
 */
export const reqHandler = createNodeRequestHandler(app);

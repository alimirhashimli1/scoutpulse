import {
  AngularNodeAppEngine,
  createNodeRequestHandler,
  isMainModule,
  writeResponseToNodeResponse,
} from '@angular/ssr/node';
import express from 'express';
import { createHash } from 'node:crypto';
import { existsSync, readFileSync } from 'node:fs';
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
 * The Content-Security-Policy, built around the page's inline scripts.
 *
 * Angular's `autoCsp` would do this, but refuses to run alongside SSR, so it is
 * done here. Scripts are allowed from this origin, from the two captcha
 * providers, and inline only by hash -- so markup that slips into a page
 * cannot run script, and a stolen session token has nowhere to be sent by a
 * script that was never allowed to load.
 *
 * Which inline scripts are trusted:
 *  - those in the built template (the theme switch and Angular's event
 *    contract), hashed once at startup. The template is build output, so
 *    nothing a request can influence is in it;
 *  - Angular's event-replay bootstrap, which lists the event types the
 *    rendered page binds and so differs page to page. It is hashed per
 *    response, and only when it matches its exact shape -- a name list and
 *    nothing else -- so an injected script cannot pass as one;
 *  - the stylesheet's `onload` swap from print to all media, by hash, through
 *    'unsafe-hashes'. Without it the global styles would stay print-only.
 *
 * `frame-ancestors` and X-Frame-Options together stop the site being framed
 * for clickjacking; the second is for browsers that predate the first.
 */
const serverDistFolder = import.meta.dirname;

const sha256 = (source: string): string =>
  `'sha256-${createHash('sha256').update(source, 'utf8').digest('base64')}'`;

/** Inline, executable scripts: not `src`, not a JSON data block. */
function inlineScripts(html: string): string[] {
  const found: string[] = [];
  for (const [, attrs, body] of html.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script>/gi)) {
    if (/\bsrc\s*=/i.test(attrs) || /type\s*=\s*["']?application\/(ld\+)?json/i.test(attrs))
      continue;
    if (body.trim()) found.push(body);
  }
  return found;
}

function templateScriptHashes(): string[] {
  const hashes = new Set<string>();
  for (const file of [
    join(serverDistFolder, 'index.server.html'),
    join(browserDistFolder, 'index.csr.html'),
  ]) {
    if (!existsSync(file)) continue;
    for (const script of inlineScripts(readFileSync(file, 'utf8'))) hashes.add(sha256(script));
  }
  if (hashes.size === 0) {
    console.warn(
      '[csp] no built template found next to the server; inline scripts will be blocked',
    );
  }
  return [...hashes];
}

const templateHashes = templateScriptHashes();
const STYLESHEET_ONLOAD = "this.media='all'";
const EVENT_REPLAY_BOOTSTRAP =
  /^window\.__jsaction_bootstrap\(document\.body,"ng",\[("[a-z]+"(,"[a-z]+")*)?\],\[("[a-z]+"(,"[a-z]+")*)?\]\);$/;
const CAPTCHA_SCRIPT_ORIGINS = [
  'https://challenges.cloudflare.com',
  'https://www.google.com/recaptcha/',
  'https://www.gstatic.com/recaptcha/',
];

function contentSecurityPolicy(html?: string): string {
  const scripts = new Set(templateHashes);
  for (const script of html ? inlineScripts(html) : []) {
    if (EVENT_REPLAY_BOOTSTRAP.test(script.trim())) scripts.add(sha256(script));
  }
  return [
    `script-src 'self' ${[...scripts].join(' ')} 'unsafe-hashes' ${sha256(STYLESHEET_ONLOAD)} ${CAPTCHA_SCRIPT_ORIGINS.join(' ')}`,
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
  ].join('; ');
}

const securityHeaders: Record<string, string> = {
  'Content-Security-Policy': contentSecurityPolicy(),
  'X-Frame-Options': 'DENY',
  'X-Content-Type-Options': 'nosniff',
  'Referrer-Policy': 'strict-origin-when-cross-origin',
  'Permissions-Policy': 'camera=(), microphone=(), geolocation=(), payment=(), usb=()',
};

app.use((_req, res, next) => {
  for (const [name, value] of Object.entries(securityHeaders)) {
    res.setHeader(name, value);
  }
  next();
});

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
    .then(async (response) => {
      if (!response) return next();
      // A rendered page carries its own event-replay bootstrap, which the
      // policy has to name. Anything else passes through untouched.
      if (!response.headers.get('content-type')?.includes('text/html')) {
        return writeResponseToNodeResponse(response, res);
      }
      const html = await response.text();
      const headers = new Headers(response.headers);
      headers.set('Content-Security-Policy', contentSecurityPolicy(html));
      return writeResponseToNodeResponse(
        new Response(html, { status: response.status, statusText: response.statusText, headers }),
        res,
      );
    })
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

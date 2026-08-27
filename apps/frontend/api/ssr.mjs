/**
 * Angular's server renderer, as a Vercel function.
 *
 * Vercel's Angular preset is a *static* build: it publishes the browser
 * directory and ignores `dist/frontend/server` entirely. With
 * `outputMode: "server"` in angular.json there is no `index.html` to publish
 * either -- Angular emits `index.csr.html` and expects a Node process to
 * answer for every route. The result was a deployment that built successfully
 * and then 404'd on every URL including `/`.
 *
 * So the renderer is wired up by hand: static assets are served from the CDN
 * (`outputDirectory` in vercel.json), and everything that is not a file on
 * disk is rewritten here.
 *
 * The server bundle is loaded through a *runtime* dynamic import rather than a
 * static one, deliberately. A static import would let esbuild inline the
 * bundle into this function, and the lazy route chunks Angular loads with
 * `import()` do not survive that intact. Keeping the specifier opaque leaves
 * the bundle on disk -- shipped by `includeFiles` -- where its own relative
 * imports resolve the way the build intended.
 */
import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const SERVER_ENTRY = 'dist/frontend/server/server.mjs';

/**
 * Where the bundle landed.
 *
 * `includeFiles` copies it in relative to the project root, which is the
 * function's working directory; the path relative to this module is the same
 * thing seen from the other end. Which of the two is correct depends on where
 * the compiler chose to emit this file, so both are tried rather than guessed
 * at -- a wrong guess here is a 500 on every page, discoverable only by
 * deploying.
 */
function resolveServerEntry() {
  const candidates = [
    new URL(`../${SERVER_ENTRY}`, import.meta.url),
    pathToFileURL(join(process.cwd(), SERVER_ENTRY)),
  ];

  for (const candidate of candidates) {
    if (existsSync(fileURLToPath(candidate))) {
      return candidate.href;
    }
  }

  throw new Error(
    `SSR bundle not found. Looked in: ${candidates.map(fileURLToPath).join(', ')}. ` +
      `Check the "includeFiles" entry for this function in vercel.json.`,
  );
}

/**
 * Hosts this deployment answers for, and the address it is published at.
 *
 * Angular validates the request host and silently falls back to client-side
 * rendering when it fails, which presents as "SSR does nothing" on a perfectly
 * healthy build. Preview deployments get a fresh hostname every time, so a
 * hand-maintained list in the project settings would be wrong for all of them
 * -- Vercel already tells the process its own hostnames, so they are read from
 * there.
 *
 * All of these are read by the bundle at module scope, so they have to be set
 * *before* the import below, not after.
 */
function applyRuntimeConfig() {
  const ownHosts = [
    process.env.VERCEL_URL,
    process.env.VERCEL_BRANCH_URL,
    process.env.VERCEL_PROJECT_PRODUCTION_URL,
  ].filter(Boolean);

  // Unioned with anything configured, never replaced by it. A deployment has
  // to answer for its own addresses -- they are the ones Vercel routes to it,
  // so they are not in question -- and making the setting additive means
  // adding a custom domain does not also mean remembering to re-list them.
  //
  // It also means a value meant for something else cannot lock the site out of
  // itself, which is not hypothetical: NG_ALLOWED_HOSTS was once set to the
  // *gateway's* hostname, and every page answered 400.
  const configured = (process.env.NG_ALLOWED_HOSTS ?? '')
    .split(',')
    .map((host) => host.trim())
    .filter(Boolean);

  process.env.NG_ALLOWED_HOSTS = [...new Set([...ownHosts, ...configured])].join(',');

  // Canonical links and og:url. The production hostname is preferred: a
  // preview naming itself canonical would point every tag at a throwaway URL.
  //
  // An explicit value wins, but only if it is absolute. These tags are required
  // to be -- a relative canonical is ignored, and a relative og:url resolves
  // against the crawler's own host -- and the failure is invisible from inside
  // the app, so a bare hostname is rejected here rather than quietly shipped in
  // the head of every page.
  const siteHost = process.env.VERCEL_PROJECT_PRODUCTION_URL ?? process.env.VERCEL_URL;
  const derived = siteHost ? `https://${siteHost}` : undefined;
  const configuredSite = process.env.SITE_URL;

  if (configuredSite && !/^https?:\/\//i.test(configuredSite)) {
    console.warn(
      '[ssr] SITE_URL=%s is not an absolute URL; ignoring it and using %s instead. ' +
        'Canonical and og:url tags must carry a scheme.',
      configuredSite,
      derived ?? '(nothing)',
    );
    if (derived) {
      process.env.SITE_URL = derived;
    }
  } else if (!configuredSite && derived) {
    process.env.SITE_URL = derived;
  }

  // Where the renderer fetches data from.
  //
  // The browser uses root-relative paths and lets the rewrites in vercel.json
  // carry them to Railway. The renderer cannot: it has no origin to resolve a
  // relative URL against, and fetch rejects one outright.
  //
  // So it is pointed at this deployment's own public origin, and goes through
  // exactly the same rewrites. That costs one extra hop through the edge per
  // fetch, and buys the thing worth having: the service addresses live in
  // vercel.json and nowhere else. The alternative -- FOOTBALL_API_URL and
  // IDENTITY_API_URL naming the Railway hosts directly -- is one hop faster and
  // two more places to forget when a URL changes.
  //
  // An explicit GATEWAY_INTERNAL_URL still wins, as does the FOOTBALL/IDENTITY
  // pair, so a deployment that wants the direct route can have it.
  const hasDirectPair = process.env.FOOTBALL_API_URL && process.env.IDENTITY_API_URL;
  if (!process.env.GATEWAY_INTERNAL_URL && !hasDirectPair && derived) {
    process.env.GATEWAY_INTERNAL_URL = derived;
  }

  // The real hostname arrives in `X-Forwarded-Host`; `Host` carries the edge's
  // own. Without this Angular checks the wrong one against the list above,
  // finds no match, and quietly serves an empty shell with a 200 -- including
  // for the routes app.routes.server.ts declares a 404 for. There is always a
  // proxy in front of a Vercel function, so this is unconditional here.
  process.env.TRUST_PROXY_HEADERS ??= 'true';

  // Cold starts are rare and this is one line of text, but it is the
  // difference between reading the answer and redeploying to guess at it.
  console.log(
    '[ssr] allowed hosts: %s | site url: %s | gateway: %s',
    process.env.NG_ALLOWED_HOSTS || '(none - system env vars not exposed?)',
    process.env.SITE_URL || '(unset)',
    process.env.GATEWAY_INTERNAL_URL || '(unset - SSR will fetch localhost and render no data)',
  );
}

/**
 * Loaded once per instance and reused, so a warm invocation does not pay to
 * re-evaluate an 800kB bundle.
 */
let renderer;

export default async function handler(request, response) {
  renderer ??= (async () => {
    applyRuntimeConfig();
    const entry = resolveServerEntry();
    const { reqHandler } = await import(entry);
    return reqHandler;
  })();

  return (await renderer)(request, response);
}

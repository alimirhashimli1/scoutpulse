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
 * Angular 21 validates the `Host` header and silently falls back to
 * client-side rendering when it fails, which presents as "SSR does nothing" on
 * a perfectly healthy build. Preview deployments get a fresh hostname every
 * time, so a hand-maintained list in the project settings would be wrong for
 * all of them -- Vercel already tells the process its own hostnames, so they
 * are read from there.
 *
 * Both are read by the bundle at module scope, so they have to be set *before*
 * the import below, not after.
 */
function applyHostConfig() {
  const hosts = [process.env.VERCEL_URL, process.env.VERCEL_PROJECT_PRODUCTION_URL].filter(Boolean);

  if (!process.env.NG_ALLOWED_HOSTS && hosts.length > 0) {
    process.env.NG_ALLOWED_HOSTS = hosts.join(',');
  }

  // Canonical links and og:url. The production hostname is preferred: a
  // preview naming itself canonical would point every tag at a throwaway URL.
  const siteHost = process.env.VERCEL_PROJECT_PRODUCTION_URL ?? process.env.VERCEL_URL;
  if (!process.env.SITE_URL && siteHost) {
    process.env.SITE_URL = `https://${siteHost}`;
  }
}

/**
 * Loaded once per instance and reused, so a warm invocation does not pay to
 * re-evaluate an 800kB bundle.
 */
let renderer;

export default async function handler(request, response) {
  renderer ??= (async () => {
    applyHostConfig();
    const entry = resolveServerEntry();
    const { reqHandler } = await import(/* @vite-ignore */ entry);
    return reqHandler;
  })();

  return (await renderer)(request, response);
}

/**
 * Removes `index.csr.html` from the directory Vercel publishes.
 *
 * Rewrites are only consulted when nothing on the filesystem matched, and
 * Vercel maps this file to `/`. So the one route that most needs the renderer
 * was answered from the CDN instead: a bare `<app-root></app-root>` with a
 * `200`, while every other route server-rendered correctly. It looked like SSR
 * worked everywhere except the home page.
 *
 * Nothing needs the file at that path. It exists for a Node process serving
 * its own static directory -- `express.static` in src/server.ts -- which is
 * not the arrangement here: assets come off the CDN, and the renderer carries
 * its own copy of the shell inside the server bundle. That copy is what
 * answers the client-rendered routes, and it is per-route (the modulepreload
 * hints differ), so it is the better answer anyway.
 *
 * Deleting it leaves `/` unmatched, which is what lets the rewrite reach the
 * function.
 */
import { rmSync, existsSync } from 'node:fs';

const shell = 'dist/frontend/browser/index.csr.html';

if (!existsSync(shell)) {
  // Not fatal -- a future Angular version may stop emitting it, and that is
  // the outcome this script is after. But say so, because the other reason it
  // is missing is that the build layout moved and this script is now silently
  // doing nothing.
  console.warn(`[vercel-postbuild] ${shell} not found; nothing to remove.`);
} else {
  rmSync(shell);
  console.log(`[vercel-postbuild] removed ${shell} so / falls through to the renderer.`);
}

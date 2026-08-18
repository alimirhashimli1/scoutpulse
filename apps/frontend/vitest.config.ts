import { defineConfig } from 'vitest/config';

/**
 * Vitest configuration for the Angular unit-test builder.
 *
 * The default `forks` pool times out starting workers on this machine —
 * Windows process spawning is slow enough, particularly with a virus scanner
 * in the path, that the worker handshake never lands. `threads` starts workers
 * in-process instead and does not hit that.
 *
 * The longer hook timeout covers the first run, where a cold TypeScript
 * compile happens inside the worker.
 */
export default defineConfig({
  test: {
    pool: 'threads',
    testTimeout: 30_000,
    hookTimeout: 60_000,
  },
});

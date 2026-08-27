# Frontend

This project was generated using [Angular CLI](https://github.com/angular/angular-cli) version 21.2.8.

## Development server

To start a local development server, run:

```bash
ng serve
```

Once the server is running, open your browser and navigate to `http://localhost:4200/`. The application will automatically reload whenever you modify any of the source files.

## Code scaffolding

Angular CLI includes powerful code scaffolding tools. To generate a new component, run:

```bash
ng generate component component-name
```

For a complete list of available schematics (such as `components`, `directives`, or `pipes`), run:

```bash
ng generate --help
```

## Building

To build the project run:

```bash
ng build
```

This will compile your project and store the build artifacts in the `dist/` directory. By default, the production build optimizes your application for performance and speed.

## Running unit tests

To execute unit tests with the [Vitest](https://vitest.dev/) test runner, use the following command:

```bash
ng test
```

## Running end-to-end tests

For end-to-end (e2e) testing, run:

```bash
ng e2e
```

Angular CLI does not come with an end-to-end testing framework by default. You can choose one that suits your needs.

## Additional Resources

For more information on using the Angular CLI, including detailed command references, visit the [Angular CLI Overview and Command Reference](https://angular.dev/tools/cli) page.

## Deploying to Vercel

The app server-renders (`outputMode: "server"` in `angular.json`), which
Vercel's `angular` framework preset cannot run. That preset performs a static
build: it publishes the browser directory and ignores `dist/frontend/server`
entirely — which in server output mode is the application. It also resolved the
output to `dist/frontend` rather than `dist/frontend/browser`, so the little it
did publish was served a level deep. The symptom was the worst kind: a build
that reported success and a site that answered `404` for every URL, `/`
included.

So the preset is off (`"framework": null`) and the pieces are named explicitly
in `vercel.json`:

| Piece | Where it goes |
| --- | --- |
| `dist/frontend/browser` | Published as static assets, served from the CDN |
| `dist/frontend/server` | Shipped into the function via `includeFiles` |
| `api/ssr.mjs` | Loads that bundle and answers everything else |

Rewrites are only consulted when nothing on the filesystem matched, so hashed
bundles never reach the function; `/:path*` matches zero segments, which is
what answers for `/`.

The two gateway prefixes are proxied to Railway one at a time rather than as a
blanket `/api/:path*`, which would also capture `/api/ssr` and send this site's
own renderer upstream.

### Environment variables

Set in Project Settings → Environment Variables, for Production and Preview:

| Variable | Value | Why |
| --- | --- | --- |
| `GATEWAY_INTERNAL_URL` | `https://scoutpulse-production.up.railway.app` | Where the *renderer* reaches the gateway. It cannot use the relative path the browser uses — a Node process has no origin to resolve one against. Without this it falls back to `http://localhost:8000` and every server-rendered page comes back with no data. |

`NG_ALLOWED_HOSTS` and `SITE_URL` are derived in `api/ssr.mjs` from the
hostnames Vercel already publishes to the process, so preview deployments —
which get a fresh hostname every time — work without a hand-maintained list.
Setting either explicitly overrides that.

`engines.node` in `package.json` pins the runtime, so it tracks the version in
`.nvmrc` and the Dockerfile rather than whatever the project setting happens to
say.

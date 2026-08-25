# The frontend server-renders, so this ships a Node process, not a static
# directory behind nginx. The placeholder this replaced was `FROM nginx:alpine`
# and a comment — it built an image that served nothing.
#
# Two stages: a build stage with the full toolchain, and a runtime carrying
# only the bundles and production dependencies. Build context is the repository
# root, so paths below are repo-relative.

# --- build ---------------------------------------------------------------
FROM node:22-alpine AS build

WORKDIR /build

# Manifests first, so `npm ci` is only re-run when dependencies actually
# change. Copying the sources first would invalidate this layer on every edit
# and reinstall the world each time.
COPY apps/frontend/package.json apps/frontend/package-lock.json ./
RUN npm ci

COPY apps/frontend/ ./
RUN npm run build

# --- runtime -------------------------------------------------------------
FROM node:22-alpine AS runtime

# node, not root. The image's own unprivileged user, so a flaw in the renderer
# is not a flaw with write access to the application.
WORKDIR /app
ENV NODE_ENV=production

# Production dependencies only: the compilers and test runner are build-time
# tools and have no business in a running container.
COPY apps/frontend/package.json apps/frontend/package-lock.json ./
RUN npm ci --omit=dev && npm cache clean --force

COPY --from=build /build/dist ./dist

USER node

# Where the *browser* reaches the gateway, baked into the client bundle at
# build time, and where *this process* reaches it, read at runtime. They are
# different network positions — see docs/FRONTEND.md §11.3.
ENV GATEWAY_INTERNAL_URL=http://gateway:8000

# The public address, for canonical links and og:url. The default suits local
# compose; set it to the real domain in any deployment that has one, or every
# canonical tag will point at localhost.
ENV SITE_URL=http://localhost:4000

# Angular 21 validates the Host header and silently falls back to client-side
# rendering when it fails — which presents as "SSR does nothing" with a working
# build. The compose service name is included so requests through the gateway
# are accepted.
ENV NG_ALLOWED_HOSTS=localhost,127.0.0.1,frontend

EXPOSE 4000

# No shell form: exec form makes the server PID 1, so a container stop is
# delivered to Node rather than to a shell that ignores it.
CMD ["node", "dist/frontend/server/server.mjs"]

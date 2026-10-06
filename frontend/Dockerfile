# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Stage 1 — build the static bundle
#
# Node is only needed here. The published image carries nothing but Nginx and
# the compiled assets, so the central server never needs a Node runtime.
# ---------------------------------------------------------------------------
FROM node:22-alpine AS build

WORKDIR /app

# The build context is the domain/ directory, not frontend/, because the Nginx
# stage below also needs deploy/docker/default.conf.template and a Dockerfile
# cannot COPY from outside its context.
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ ./

# `npm run build` runs vue-tsc before vite, so a type error fails the image
# build rather than shipping a broken dashboard to the wall.
RUN npm run build


# ---------------------------------------------------------------------------
# Stage 2 — serve
# ---------------------------------------------------------------------------
FROM nginx:1.27-alpine

# The official image renders every *.template through envsubst at startup,
# which is how the agent address from .env reaches the proxy without a rebuild.
COPY deploy/docker/default.conf.template /etc/nginx/templates/default.conf.template

COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q --spider http://127.0.0.1/ || exit 1

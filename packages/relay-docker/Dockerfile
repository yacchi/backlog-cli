# Unified container image: Backlog OAuth Relay + MCP server.
#
# Self-contained multi-stage build (context = repo root):
#   docker build -f packages/relay-docker/Dockerfile -t backlog-relay .
#
# The same image runs as a plain HTTP server (docker run) and as an AWS Lambda
# container image (via the bundled Lambda Web Adapter).
#
# Built for linux/arm64 (matches the Lambda arm64 plan). On an amd64 host use:
#   docker buildx build --platform linux/arm64 -f packages/relay-docker/Dockerfile .

# ---- Node: install workspace and build the unified runtime + portal ----
FROM node:24-slim AS node-builder
RUN npm install -g pnpm@9
WORKDIR /repo
COPY . .
RUN pnpm install --frozen-lockfile
# relay-core / mcp-server must be built first; tsup bundles them (and the AWS SDK)
# into a single self-contained relay-docker/dist/index.js. `web` builds the portal SPA.
RUN pnpm --filter @backlog-cli/relay-core build \
 && pnpm --filter @backlog-cli/mcp-server build \
 && pnpm --filter @backlog-cli/relay-docker build \
 && pnpm --filter web build

# ---- Go: build the backlog CLI binary ----
# The Go module lives at the repo root (module github.com/yacchi/backlog-cli);
# cmd/backlog imports internal/ and packages/backlog/, so copy the whole context
# (node_modules/dist are excluded via .dockerignore).
FROM golang:1.25 AS go-builder
WORKDIR /src
COPY . .
# cmd/backlog embeds the portal SPA via packages/web/embed.go (`//go:embed all:dist`),
# so the built web/dist must be present before `go build`. Bring it from node-builder.
COPY --from=node-builder /repo/packages/web/dist ./packages/web/dist
# GOWORK=off ignores the repo's go.work (which points at a local ../jubako for
# dev) so the build resolves the published jubako version from go.mod instead.
RUN GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /out/backlog ./cmd/backlog

# ---- Deno: compile the sandbox worker to a standalone binary ----
# SECURITY: bake ONLY --allow-net=127.0.0.1 into the compiled binary. deno compile
# embeds the Pyodide wasm/stdlib into the binary's virtual filesystem, so no
# --allow-read/--allow-write is needed at runtime. Granting either would let a
# submitted Python script escape the Pyodide sandbox via `import js` (which exposes
# the full Deno API) and read files such as /proc/self/environ. Net is scoped to
# loopback for the IPC callback to the Node parent.
FROM denoland/deno:latest AS deno-builder
WORKDIR /w
COPY packages/mcp-server/src/sandbox/sandbox-worker.mjs .
# Use deno's bundled Mozilla CA store (the default) — the deno image has no
# populated system CA store, so DENO_TLS_CA_STORE=system would fail npm fetches.
RUN deno compile \
      --target aarch64-unknown-linux-gnu \
      --allow-net=127.0.0.1 \
      --output /out/sandbox-worker \
      sandbox-worker.mjs

# ---- Final runtime image ----
# Distroless (debian12-based): no shell, no apt, no package manager — smaller and
# a reduced attack surface. debian12 provides glibc, so the glibc-dynamic
# deno-compiled sandbox worker and the static Go binary both run. The image's
# ENTRYPOINT is node, so CMD carries only the script path.
FROM gcr.io/distroless/nodejs24-debian12
# Lambda Web Adapter: makes the same HTTP server runnable as a Lambda container.
COPY --from=public.ecr.aws/awsguru/aws-lambda-adapter:1.0.1 /lambda-adapter /opt/extensions/lambda-adapter

# Link the GHCR package to the backlog-cli repository (appears under the repo's
# Packages and inherits its permissions). GHCR reads this label on push.
LABEL org.opencontainers.image.source="https://github.com/yacchi/backlog-cli" \
      org.opencontainers.image.title="backlog-relay" \
      org.opencontainers.image.description="Backlog OAuth Relay + MCP server (unified runtime)"

WORKDIR /app

# Self-contained Node bundle (relay-core + mcp-server + AWS SDK bundled by tsup).
COPY --from=node-builder /repo/packages/relay-docker/dist /app/dist
# Portal SPA assets.
COPY --from=node-builder /repo/packages/web/dist /app/web/dist
# Runtime helper binaries.
COPY --from=go-builder /out/backlog /app/bin/backlog
COPY --from=deno-builder /out/sandbox-worker /app/bin/sandbox-worker

ENV NODE_ENV=production
ENV PORT=8080
ENV WEB_DIST_PATH=/app/web/dist
ENV BACKLOG_BIN_PATH=/app/bin/backlog
ENV SANDBOX_WORKER_PATH=/app/bin/sandbox-worker

EXPOSE 8080
# ENTRYPOINT is node (provided by the distroless image).
CMD ["dist/index.js"]

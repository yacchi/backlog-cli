FROM golang:1.23 AS go-builder
WORKDIR /build
COPY packages/backlog/ .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o backlog ./cmd/backlog

FROM node:22-slim AS node-builder
WORKDIR /app
COPY packages/mcp-server/package.json packages/mcp-server/pnpm-lock.yaml* ./
RUN npm ci --omit=dev 2>/dev/null || npm install --omit=dev
COPY packages/mcp-server/ .
RUN npm run build

FROM node:22-slim
COPY --from=public.ecr.aws/awsguru/aws-lambda-web-adapter:0.9.1 /lambda-adapter /opt/extensions/lambda-adapter

WORKDIR /app
COPY --from=go-builder /build/backlog /app/bin/backlog
COPY --from=node-builder /app/dist /app/dist
COPY --from=node-builder /app/node_modules /app/node_modules
COPY --from=node-builder /app/src/sandbox/sandbox-worker.mjs /app/sandbox-worker.mjs

ENV PORT=8080
ENV BACKLOG_BIN_PATH=/app/bin/backlog
EXPOSE 8080
CMD ["node", "dist/serve.js"]

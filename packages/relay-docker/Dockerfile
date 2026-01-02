# Build stage
FROM node:22-alpine AS builder

WORKDIR /app

# Install pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate

# Copy workspace files
COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY packages/relay-core/package.json packages/relay-core/
COPY packages/relay-docker/package.json packages/relay-docker/

# Install dependencies
RUN pnpm install --frozen-lockfile

# Copy source files
COPY packages/relay-core/ packages/relay-core/
COPY packages/relay-docker/ packages/relay-docker/

# Build
RUN pnpm --filter @backlog-cli/relay-core build
RUN pnpm --filter @backlog-cli/relay-docker build

# Production stage
FROM node:22-alpine AS runner

WORKDIR /app

# Install pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate

# Copy workspace files
COPY pnpm-workspace.yaml package.json ./
COPY packages/relay-core/package.json packages/relay-core/
COPY packages/relay-docker/package.json packages/relay-docker/

# Copy built files
COPY --from=builder /app/packages/relay-core/dist packages/relay-core/dist/
COPY --from=builder /app/packages/relay-docker/dist packages/relay-docker/dist/

# Install production dependencies only
RUN pnpm install --prod --frozen-lockfile

# Set environment
ENV NODE_ENV=production
ENV PORT=3000

EXPOSE 3000

# Run the server
CMD ["node", "packages/relay-docker/dist/index.js"]

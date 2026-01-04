# Simple production image
# All dependencies are bundled locally, no npm install needed
FROM node:22-alpine

WORKDIR /app

# Copy pre-bundled application
COPY dist/index.js ./

# Copy web assets
COPY public/ ./public/

# Set environment
ENV NODE_ENV=production
ENV PORT=3000
ENV WEB_DIST_PATH=/app/public

EXPOSE 3000

# Run the bundled server
CMD ["node", "index.js"]

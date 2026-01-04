# Simple production image
# All dependencies are bundled locally, no npm install needed
FROM node:22-alpine

WORKDIR /app

# Copy pre-built dist directory (includes index.js and public/)
COPY dist/ ./

# Set environment
ENV NODE_ENV=production
ENV PORT=3000
ENV WEB_DIST_PATH=/app/public

EXPOSE 3000

# Run the bundled server
CMD ["node", "index.js"]

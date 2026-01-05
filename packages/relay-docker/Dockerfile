# Minimal production image using distroless
# All dependencies are bundled locally, no shell or package manager needed
FROM gcr.io/distroless/nodejs24-debian12

WORKDIR /app

# Copy pre-built dist directory (includes index.js and public/)
COPY dist/ ./

# Set environment
ENV NODE_ENV=production
ENV WEB_DIST_PATH=/app/public

# Default port (configurable via RELAY_CONFIG server.port)
EXPOSE 8080

# Run the bundled server
CMD ["index.js"]

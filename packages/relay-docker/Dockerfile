# Minimal production image using distroless
# All dependencies are bundled locally, no shell or package manager needed
FROM gcr.io/distroless/nodejs24-debian12

WORKDIR /app

# Copy pre-built dist directory (includes index.js and public/)
COPY dist/ ./

# Set environment
ENV NODE_ENV=production
ENV PORT=3000
ENV WEB_DIST_PATH=/app/public

EXPOSE 3000

# Run the bundled server
CMD ["index.js"]

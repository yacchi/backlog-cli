.PHONY: build dev clean install

# Output marker for dependency tracking
DIST_INDEX := dist/index.html

# Source files for dependency tracking
SOURCES := $(shell find src -type f 2>/dev/null)

# Build (incremental)
$(DIST_INDEX): $(SOURCES) package.json
	pnpm install --frozen-lockfile && pnpm build

build: $(DIST_INDEX)

# Force rebuild
.PHONY: build-force
build-force:
	@rm -f $(DIST_INDEX)
	@$(MAKE) build

# Development server
dev:
	pnpm dev

# Install dependencies
install:
	pnpm install --frozen-lockfile

# Clean
clean:
	rm -rf dist node_modules/.vite

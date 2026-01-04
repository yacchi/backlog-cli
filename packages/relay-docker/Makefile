.PHONY: build dev clean check-native docker

# Paths
ROOT_DIR := ../..
WEB_DIST_DIR := $(ROOT_DIR)/packages/web/dist
IMAGE_NAME := backlog-relay

# Build everything for Docker
build: bundle assets check-native docker

# Bundle application (all dependencies included)
.PHONY: bundle
bundle:
	$(MAKE) -C $(ROOT_DIR) build-relay-core
	pnpm build

# Copy web assets to dist/public
.PHONY: assets
assets:
	$(MAKE) -C $(ROOT_DIR) build-web
	@mkdir -p dist/public
	rsync -av --delete $(WEB_DIST_DIR)/ dist/public/

# Check for native modules
check-native:
	@bash scripts/check-native-modules.sh

# Build Docker image (context is this directory)
docker:
	docker build -t $(IMAGE_NAME) .

# Development server
dev:
	$(MAKE) -C $(ROOT_DIR) build-web
	$(MAKE) -C $(ROOT_DIR) build-relay-core
	pnpm dev

# Clean
clean:
	rm -rf dist

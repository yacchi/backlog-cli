.PHONY: build dev clean

# Paths
ROOT_DIR := ../..
IMAGE_NAME := backlog-relay

# Build Docker image
# Dockerfile はルートのコンテキストでビルドする必要がある
build:
	$(MAKE) -C $(ROOT_DIR) build-relay-core
	pnpm build
	docker build -t $(IMAGE_NAME) -f Dockerfile $(ROOT_DIR)

# Development server (with web assets)
dev:
	$(MAKE) -C $(ROOT_DIR) build-web
	$(MAKE) -C $(ROOT_DIR) build-relay-core
	pnpm dev

# Clean
clean:
	rm -rf dist

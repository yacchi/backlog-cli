.PHONY: build dev clean docker check-native

# Paths
ROOT_DIR := ../..
IMAGE_NAME := backlog-relay
# Self-contained multi-stage Dockerfile; build context is the repo root.
DOCKERFILE := packages/relay-docker/Dockerfile

# Build the unified container image. The multi-stage build compiles the Go CLI,
# the Deno sandbox worker, the Node bundle, and the portal entirely inside Docker.
build: docker

docker:
	docker build -f $(DOCKERFILE) -t $(IMAGE_NAME) $(ROOT_DIR)

# Optional: detect native modules that tsup cannot bundle into a single file.
check-native:
	@bash scripts/check-native-modules.sh

# Local dev server (host build, no container). Requires RELAY_CONFIG in the env.
dev:
	$(MAKE) -C $(ROOT_DIR) build-web
	$(MAKE) -C $(ROOT_DIR) build-relay-core
	pnpm dev

# Clean
clean:
	rm -rf dist

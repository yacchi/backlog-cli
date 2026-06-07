.PHONY: build deploy synth assets copy-assets deps clean

# Paths
ROOT_DIR := ../..
WEB_DIST_DIR := $(ROOT_DIR)/packages/web/dist
LOCAL_ASSETS_DIR := dist/web-assets
RELAY_CORE_DIR := $(ROOT_DIR)/packages/relay-core
MCP_SERVER_DIR := $(ROOT_DIR)/packages/mcp-server

# Build: prepare assets for CDK bundling
build: assets

# Build workspace dependencies (relay-core, mcp-server)
# Required because esbuild resolves these packages via dist/ (compiled JS),
# not TypeScript source. Without this, source changes are not picked up.
deps:
	pnpm --filter @backlog-cli/relay-core --filter @backlog-cli/mcp-server build

# Copy web assets to local staging directory
assets:
	$(MAKE) -C $(ROOT_DIR) build-web
	@mkdir -p $(LOCAL_ASSETS_DIR)
	rsync -av --delete $(WEB_DIST_DIR)/ $(LOCAL_ASSETS_DIR)/

# Copy assets to Lambda bundle output directory
# Called by CDK afterBundling with OUTPUT_DIR=<full-path>
# Depends on assets to ensure they are built first
copy-assets: assets
ifndef OUTPUT_DIR
	$(error OUTPUT_DIR is required)
endif
	cp -r $(LOCAL_ASSETS_DIR) $(OUTPUT_DIR)

# CDK synth (includes asset + deps build)
synth: deps assets
	pnpm cdk synth BacklogOAuthRelay

# CDK deploy (includes asset + deps build)
deploy: deps assets
	pnpm cdk deploy BacklogOAuthRelay

# Clean
clean:
	rm -rf dist cdk.out

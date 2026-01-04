.PHONY: build deploy synth assets copy-assets clean

# Paths
ROOT_DIR := ../..
WEB_DIST_DIR := $(ROOT_DIR)/packages/web/dist
LOCAL_WEB_DIST := dist/web-dist

# Build: prepare assets for CDK bundling
build: assets

# Copy web assets to local dist/web-dist
assets:
	$(MAKE) -C $(ROOT_DIR) build-web
	@mkdir -p $(LOCAL_WEB_DIST)
	rsync -av --delete $(WEB_DIST_DIR)/ $(LOCAL_WEB_DIST)/

# Copy assets to Lambda bundle output directory
# Called by CDK afterBundling with OUTPUT_DIR=<path>
# Depends on assets to ensure they are built first
copy-assets: assets
ifndef OUTPUT_DIR
	$(error OUTPUT_DIR is required. Usage: make copy-assets OUTPUT_DIR=/path/to/output)
endif
	cp -r $(LOCAL_WEB_DIST) $(OUTPUT_DIR)/web-dist

# CDK synth (includes asset build)
synth: assets
	pnpm cdk synth

# CDK deploy (includes asset build)
deploy: assets
	pnpm cdk deploy

# Clean
clean:
	rm -rf dist cdk.out

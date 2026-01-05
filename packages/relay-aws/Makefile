.PHONY: build deploy synth assets copy-assets clean

# Paths
ROOT_DIR := ../..
WEB_DIST_DIR := $(ROOT_DIR)/packages/web/dist
LOCAL_ASSETS_DIR := dist/web-assets

# Build: prepare assets for CDK bundling
build: assets

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

# CDK synth (includes asset build)
synth: assets
	pnpm cdk synth

# CDK deploy (includes asset build)
deploy: assets
	pnpm cdk deploy

# Clean
clean:
	rm -rf dist cdk.out

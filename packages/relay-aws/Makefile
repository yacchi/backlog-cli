.PHONY: build deploy synth assets clean

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

# CDK synth (includes asset build)
synth: assets
	pnpm cdk synth

# CDK deploy (includes asset build)
deploy: assets
	pnpm cdk deploy

# Clean
clean:
	rm -rf dist cdk.out

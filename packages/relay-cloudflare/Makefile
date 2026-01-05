.PHONY: assets deploy dev clean

# Paths
ROOT_DIR := ../..
PUBLIC_DIR := public
WEB_DIST_DIR := $(ROOT_DIR)/packages/web/dist

# Build web assets and copy to public/
assets:
	$(MAKE) -C $(ROOT_DIR) build-web
	@mkdir -p $(PUBLIC_DIR)
	rsync -av --delete $(WEB_DIST_DIR)/ $(PUBLIC_DIR)/

# Deploy to Cloudflare Workers
deploy: assets
	pnpm run deploy

# Local development
dev: assets
	pnpm run dev

# Clean
clean:
	rm -rf $(PUBLIC_DIR)

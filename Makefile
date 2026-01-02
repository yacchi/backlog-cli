.PHONY: build test lint clean run serve install build-web dev-web build-dev buf-generate buf-lint

# バージョン情報
VERSION ?= $(shell cat version.txt 2>/dev/null | tr -d '[:space:]' || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# ビルドフラグ
LDFLAGS := -ldflags "-s -w \
	-X github.com/yacchi/backlog-cli/packages/backlog/internal/cmd.Version=$(VERSION) \
	-X github.com/yacchi/backlog-cli/packages/backlog/internal/cmd.Commit=$(COMMIT) \
	-X github.com/yacchi/backlog-cli/packages/backlog/internal/cmd.BuildDate=$(BUILD_DATE)"

# 出力先
BUILD_DIR := ./build
BINARY := backlog

# デフォルトターゲット
all: build

# ビルド
build: build-web
	@mkdir -p $(BUILD_DIR)
	go generate ./...
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/backlog

# 開発用実行（go run使用）
run:
	go run ./cmd/backlog $(ARGS)

# テスト (-race requires CGO)
test:
	CGO_ENABLED=1 go test -v -race -cover ./...

# カバレッジ
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# リント (golangci-lint が必要)
lint:
	golangci-lint run ./...

# フォーマット
fmt:
	go fmt ./...
	goimports -w .

# 依存関係の整理
tidy:
	go mod tidy

# OpenAPI コード生成
generate:
	go tool ogen --target packages/backlog/internal/backlog --clean --package backlog api/openapi.yaml
	@echo "Applying post-generation fixes for null handling..."
	@./scripts/fix-ogen-null.sh

# OpenAPI コード生成（修正なし、デバッグ用）
generate-raw:
	go tool ogen --target packages/backlog/internal/backlog --clean --package backlog api/openapi.yaml

# クリーン
clean:
	rm -rf $(BUILD_DIR)
	rm -f $(BINARY)
	rm -f coverage.out coverage.html
	rm -rf packages/web/dist packages/web/node_modules/.vite

# Temporary directory for stamps
TMP_DIR := .tmp

# Proto sources
PROTO_SOURCES := $(shell find proto -name '*.proto' 2>/dev/null)
BUF_CONFIG := buf.yaml buf.gen.yaml

# Stamp file for buf generate
GEN_STAMP := $(TMP_DIR)/.buf-generate-stamp

# Generate proto files only when sources change
$(GEN_STAMP): $(PROTO_SOURCES) $(BUF_CONFIG)
	mise exec -- buf generate
	rm -rf packages/web/src/gen
	cp -r gen/ts packages/web/src/gen
	cd packages/web && pnpm install --frozen-lockfile && pnpm exec prettier --write src/gen/
	@mkdir -p $(TMP_DIR)
	@touch $@

# Proto コード生成（変更時のみ実行）
buf-generate: $(GEN_STAMP)

# 強制的に再生成
.PHONY: buf-generate-force
buf-generate-force:
	@rm -f $(GEN_STAMP)
	@$(MAKE) buf-generate

# Proto lint
buf-lint:
	mise exec -- buf lint

# フロントエンドビルド（packages/webにビルド結果が配置される）
build-web: buf-generate
	cd packages/web && pnpm install --frozen-lockfile && pnpm build

# フロントエンド開発サーバー
dev-web:
	cd packages/web && pnpm dev

# 開発用ビルド（フロントエンドビルドをスキップ）
build-dev:
	@mkdir -p $(BUILD_DIR)
	go build -tags=dev $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/backlog

# 中継サーバー起動（開発用、go run使用）
serve:
	go run ./cmd/backlog serve $(ARGS)

# インストール
install: build
	cp $(BUILD_DIR)/$(BINARY) $(GOBIN)/$(BINARY)

# クロスコンパイル
.PHONY: build-all
build-all:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-amd64 ./cmd/backlog
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 ./cmd/backlog
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/backlog
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-arm64 ./cmd/backlog
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-windows-amd64.exe ./cmd/backlog

# ==== 中継サーバー (TypeScript) ====

# 中継サーバー共通ライブラリのビルド
.PHONY: build-relay-core
build-relay-core:
	pnpm --filter @backlog-cli/relay-core build

# 中継サーバー（Docker）
.PHONY: build-relay-docker
build-relay-docker: build-relay-core
	pnpm --filter @backlog-cli/relay-docker build
	docker build -t backlog-relay packages/relay-docker

# 中継サーバー（Cloudflare Workers）
.PHONY: deploy-relay-cf
deploy-relay-cf: build-relay-core
	pnpm --filter @backlog-cli/relay-cloudflare deploy

# 中継サーバー（AWS Lambda）
.PHONY: build-relay-aws
build-relay-aws: build-relay-core
	pnpm --filter @backlog-cli/relay-aws build

.PHONY: deploy-relay-aws
deploy-relay-aws: build-relay-aws
	pnpm --filter @backlog-cli/relay-aws deploy

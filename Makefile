.PHONY: build test lint clean run serve install build-web dev-web build-dev

# バージョン情報
VERSION ?= $(shell cat version.txt 2>/dev/null | tr -d '[:space:]' || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# ビルドフラグ
LDFLAGS := -ldflags "-s -w \
	-X github.com/yacchi/backlog-cli/internal/cmd.Version=$(VERSION) \
	-X github.com/yacchi/backlog-cli/internal/cmd.Commit=$(COMMIT) \
	-X github.com/yacchi/backlog-cli/internal/cmd.BuildDate=$(BUILD_DATE)"

# 出力先
BUILD_DIR := ./build
BINARY := backlog

# デフォルトターゲット
all: build

# ビルド
build: build-web
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/backlog

# 開発用実行（go run使用）
run:
	go run ./cmd/backlog $(ARGS)

# テスト
test:
	go test -v -race -cover ./...

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
	go tool ogen --target internal/backlog --clean --package backlog api/openapi.yaml
	@echo "Applying post-generation fixes for null handling..."
	@./scripts/fix-ogen-null.sh

# OpenAPI コード生成（修正なし、デバッグ用）
generate-raw:
	go tool ogen --target internal/backlog --clean --package backlog api/openapi.yaml

# クリーン
clean:
	rm -rf $(BUILD_DIR)
	rm -f $(BINARY)
	rm -f coverage.out coverage.html
	rm -rf internal/ui/dist web/node_modules/.vite

# フロントエンドビルド
build-web:
	cd web && pnpm install && pnpm build
	rm -rf internal/ui/dist
	cp -r web/dist internal/ui/dist

# フロントエンド開発サーバー
dev-web:
	cd web && pnpm dev

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

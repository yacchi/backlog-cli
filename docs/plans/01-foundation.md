# Phase 01: 基盤構築

## 目標

- プロジェクトの基本構造を作成
- mise によるツール管理
- Makefile によるビルド環境
- Cobra によるCLIフレームワーク

## 1. mise 設定

### .mise.toml

```toml
[tools]
go = "1.23"

[env]
GOBIN = "{{env.HOME}}/.local/bin"
CGO_ENABLED = "0"
```

## 2. Makefile

### Makefile

```makefile
.PHONY: build test lint clean dev serve install

# バージョン情報
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# ビルドフラグ
LDFLAGS := -ldflags "-s -w \
	-X github.com/yourorg/backlog-cli/internal/cmd.Version=$(VERSION) \
	-X github.com/yourorg/backlog-cli/internal/cmd.Commit=$(COMMIT) \
	-X github.com/yourorg/backlog-cli/internal/cmd.BuildDate=$(BUILD_DATE)"

# 出力先
BUILD_DIR := ./build
BINARY := backlog

# デフォルトターゲット
all: build

# ビルド
build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/backlog

# 開発用ビルド（最適化なし）
dev:
	go build -o $(BINARY) ./cmd/backlog

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

# クリーン
clean:
	rm -rf $(BUILD_DIR)
	rm -f $(BINARY)
	rm -f coverage.out coverage.html

# 中継サーバー起動（開発用）
serve: dev
	./$(BINARY) serve

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
```

## 3. Go モジュール初期化

```bash
go mod init github.com/yourorg/backlog-cli
```

### go.mod (初期)

```go
module github.com/yourorg/backlog-cli

go 1.23

require (
	github.com/spf13/cobra v1.8.0
	gopkg.in/yaml.v3 v3.0.1
	github.com/AlecAivazis/survey/v2 v2.3.7
	github.com/golang-jwt/jwt/v5 v5.2.0
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
)
```

## 4. ディレクトリ構造作成

```bash
mkdir -p cmd/backlog
mkdir -p internal/cmd/auth
mkdir -p internal/cmd/issue
mkdir -p internal/cmd/pr
mkdir -p internal/cmd/project
mkdir -p internal/cmd/wiki
mkdir -p internal/cmd/config
mkdir -p internal/cmd/serve
mkdir -p internal/api
mkdir -p internal/config
mkdir -p internal/auth
mkdir -p internal/relay
mkdir -p internal/ui
```

## 5. エントリーポイント

### cmd/backlog/main.go

```go
package main

import (
	"os"

	"github.com/yourorg/backlog-cli/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

## 6. ルートコマンド

### internal/cmd/root.go

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "backlog",
	Short: "Backlog CLI - A command line interface for Backlog",
	Long: `Backlog CLI brings Backlog to your terminal.

Work with issues, pull requests, wikis, and more, all from the command line.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// グローバルフラグ
	rootCmd.PersistentFlags().StringP("project", "p", "", "Backlog project key")
	rootCmd.PersistentFlags().StringP("space", "s", "", "Backlog space name")
	rootCmd.PersistentFlags().String("domain", "", "Backlog domain (backlog.jp or backlog.com)")
	rootCmd.PersistentFlags().StringP("output", "o", "", "Output format (table, json)")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
}
```

### internal/cmd/version.go

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("backlog version %s\n", Version)
		fmt.Printf("  commit: %s\n", Commit)
		fmt.Printf("  built:  %s\n", BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
```

## 7. サブコマンドのスケルトン

### internal/cmd/auth/auth.go

```go
package auth

import (
	"github.com/spf13/cobra"
)

var AuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with Backlog",
	Long:  "Manage authentication state for Backlog CLI.",
}

func init() {
	AuthCmd.AddCommand(loginCmd)
	AuthCmd.AddCommand(logoutCmd)
	AuthCmd.AddCommand(statusCmd)
	AuthCmd.AddCommand(setupCmd)
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to Backlog",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: 実装
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from Backlog",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: 実装
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: 実装
		return nil
	},
}

var setupCmd = &cobra.Command{
	Use:   "setup <relay-server-url>",
	Short: "Configure relay server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: 実装
		return nil
	},
}
```

### internal/cmd/issue/issue.go

```go
package issue

import (
	"github.com/spf13/cobra"
)

var IssueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Manage Backlog issues",
	Long:  "Work with Backlog issues.",
}

func init() {
	IssueCmd.AddCommand(listCmd)
	IssueCmd.AddCommand(viewCmd)
	IssueCmd.AddCommand(createCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: 実装
		return nil
	},
}

var viewCmd = &cobra.Command{
	Use:   "view <issue-key>",
	Short: "View an issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: 実装
		return nil
	},
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new issue",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: 実装
		return nil
	},
}
```

## 8. サブコマンドの登録

### internal/cmd/root.go に追加

```go
import (
	// ...
	"github.com/yourorg/backlog-cli/internal/cmd/auth"
	"github.com/yourorg/backlog-cli/internal/cmd/issue"
	// 他のサブコマンドも同様に追加
)

func init() {
	// ... 既存のフラグ設定

	// サブコマンド登録
	rootCmd.AddCommand(auth.AuthCmd)
	rootCmd.AddCommand(issue.IssueCmd)
	// 他のサブコマンドも同様に追加
}
```

## 9. .gitignore

```gitignore
# Binaries
/build/
/backlog
*.exe

# Test coverage
coverage.out
coverage.html

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Go
vendor/

# Config (ローカルテスト用)
config.yaml
.env
```

## 完了条件

- [ ] `mise install` でGoがインストールされる
- [ ] `make build` でバイナリがビルドされる
- [ ] `./build/backlog version` でバージョン情報が表示される
- [ ] `./build/backlog --help` でヘルプが表示される
- [ ] `./build/backlog auth --help` でauthサブコマンドのヘルプが表示される
- [ ] `./build/backlog issue --help` でissueサブコマンドのヘルプが表示される

## 次のステップ

`02-config.md` に進んで設定管理を実装してください。

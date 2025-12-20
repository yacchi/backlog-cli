# Phase 10: 改善

## 目標

- エラーハンドリングの統一
- シェル補完
- ドキュメント整備
- テスト追加
- リリース準備

## 1. エラーハンドリング統一

### internal/cmd/errors.go

```go
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/yourorg/backlog-cli/internal/api"
	"github.com/yourorg/backlog-cli/internal/ui"
)

// ExitCode はエラーの終了コード
type ExitCode int

const (
	ExitOK        ExitCode = 0
	ExitError     ExitCode = 1
	ExitAuth      ExitCode = 2
	ExitNotFound  ExitCode = 3
	ExitConfig    ExitCode = 4
)

// HandleError はエラーを処理して適切なメッセージを表示する
func HandleError(err error) ExitCode {
	if err == nil {
		return ExitOK
	}
	
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return handleAPIError(apiErr)
	}
	
	// 一般的なエラー
	ui.Error("%v", err)
	return ExitError
}

func handleAPIError(err *api.APIError) ExitCode {
	switch err.StatusCode {
	case 401:
		ui.Error("Authentication required. Run 'backlog auth login' to authenticate.")
		return ExitAuth
	case 403:
		ui.Error("Permission denied: %s", getErrorMessage(err))
		return ExitAuth
	case 404:
		ui.Error("Not found: %s", getErrorMessage(err))
		return ExitNotFound
	case 429:
		ui.Error("Rate limit exceeded. Please wait and try again.")
		return ExitError
	default:
		ui.Error("API error (%d): %s", err.StatusCode, getErrorMessage(err))
		return ExitError
	}
}

func getErrorMessage(err *api.APIError) string {
	if len(err.Errors) > 0 {
		return err.Errors[0].Message
	}
	return fmt.Sprintf("status %d", err.StatusCode)
}
```

### cmd/backlog/main.go 更新

```go
package main

import (
	"os"

	"github.com/yourorg/backlog-cli/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		exitCode := cmd.HandleError(err)
		os.Exit(int(exitCode))
	}
}
```

## 2. シェル補完

### internal/cmd/completion.go

```go
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate completion script",
	Long: `Generate shell completion script.

To load completions:

Bash:
  $ source <(backlog completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ backlog completion bash > /etc/bash_completion.d/backlog
  # macOS:
  $ backlog completion bash > $(brew --prefix)/etc/bash_completion.d/backlog

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc
  
  # To load completions for each session, execute once:
  $ backlog completion zsh > "${fpath[1]}/_backlog"
  
  # You will need to start a new shell for this setup to take effect.

Fish:
  $ backlog completion fish | source
  # To load completions for each session, execute once:
  $ backlog completion fish > ~/.config/fish/completions/backlog.fish

PowerShell:
  PS> backlog completion powershell | Out-String | Invoke-Expression
  # To load completions for every new session, run:
  PS> backlog completion powershell > backlog.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
```

### カスタム補完

```go
// internal/cmd/issue/issue.go に追加

func init() {
	// 課題キーの補完
	viewCmd.RegisterFlagCompletionFunc("", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// 最近の課題を取得して補完候補にする
		// 実装省略
		return nil, cobra.ShellCompDirectiveNoFileComp
	})
}
```

## 3. テスト追加

### internal/api/client_test.go

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetCurrentUser(t *testing.T) {
	// モックサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/users/myself" {
			t.Errorf("Expected path /api/v2/users/myself, got %s", r.URL.Path)
		}
		
		// Authorization ヘッダーチェック
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Expected Authorization: Bearer test-token, got %s", auth)
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id": 1, "userId": "test", "name": "Test User"}`))
	}))
	defer server.Close()
	
	// テスト用クライアント（baseURL をモックサーバーに差し替え）
	client := &Client{
		accessToken: "test-token",
		httpClient:  http.DefaultClient,
	}
	// テスト用にbaseURLをオーバーライドする仕組みが必要
	
	// テスト省略（実際の実装では依存性注入を使う）
}

func TestCheckResponse(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    bool
	}{
		{
			name:       "200 OK",
			statusCode: 200,
			body:       "{}",
			wantErr:    false,
		},
		{
			name:       "401 Unauthorized",
			statusCode: 401,
			body:       `{"errors":[{"message":"Authentication required","code":11}]}`,
			wantErr:    true,
		},
		{
			name:       "404 Not Found",
			statusCode: 404,
			body:       `{"errors":[{"message":"Issue not found","code":6}]}`,
			wantErr:    true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tt.statusCode,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}
			
			err := CheckResponse(resp)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

### internal/config/config_test.go

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve(t *testing.T) {
	cfg := &Config{
		Client: ClientConfig{
			Default: ClientDefaultConfig{
				Space:   "default-space",
				Domain:  "backlog.jp",
				Project: "DEFAULT",
			},
		},
	}
	
	tests := []struct {
		name    string
		opts    ResolveOptions
		want    *ResolvedConfig
	}{
		{
			name: "use defaults",
			opts: ResolveOptions{},
			want: &ResolvedConfig{
				Space:   "default-space",
				Domain:  "backlog.jp",
				Project: "DEFAULT",
			},
		},
		{
			name: "override with options",
			opts: ResolveOptions{
				Space:   "other-space",
				Project: "OTHER",
			},
			want: &ResolvedConfig{
				Space:   "other-space",
				Domain:  "backlog.jp",
				Project: "OTHER",
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(cfg, tt.opts)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			
			if got.Space != tt.want.Space {
				t.Errorf("Space = %v, want %v", got.Space, tt.want.Space)
			}
			if got.Domain != tt.want.Domain {
				t.Errorf("Domain = %v, want %v", got.Domain, tt.want.Domain)
			}
			if got.Project != tt.want.Project {
				t.Errorf("Project = %v, want %v", got.Project, tt.want.Project)
			}
		})
	}
}
```

## 4. ドキュメント整備

### README.md

```markdown
# Backlog CLI

A command-line interface for [Backlog](https://backlog.com/) project management.

## Installation

### Using Homebrew (macOS/Linux)

```bash
brew install yourorg/tap/backlog-cli
```

### Using Go

```bash
go install github.com/yourorg/backlog-cli/cmd/backlog@latest
```

### Binary Releases

Download from [GitHub Releases](https://github.com/yourorg/backlog-cli/releases).

## Quick Start

1. Set up the relay server (or use a hosted one):
   ```bash
   backlog auth setup https://relay.example.com
   ```

2. Log in to Backlog:
   ```bash
   backlog auth login
   ```

3. Initialize your project:
   ```bash
   backlog project init
   ```

4. Start using:
   ```bash
   backlog issue list
   backlog issue create
   ```

## Commands

### Authentication
- `backlog auth login` - Log in to Backlog
- `backlog auth logout` - Log out
- `backlog auth status` - Show authentication status
- `backlog auth setup` - Configure relay server

### Issues
- `backlog issue list` - List issues
- `backlog issue view <key>` - View an issue
- `backlog issue create` - Create a new issue
- `backlog issue edit <key>` - Edit an issue
- `backlog issue close <key>` - Close an issue
- `backlog issue comment <key>` - Add a comment

### Projects
- `backlog project list` - List projects
- `backlog project view [key]` - View project details
- `backlog project init` - Create .backlog.yaml

### Pull Requests
- `backlog pr list` - List pull requests
- `backlog pr view <number>` - View a pull request

### Wiki
- `backlog wiki list` - List wiki pages
- `backlog wiki view <id>` - View a wiki page
- `backlog wiki create` - Create a wiki page

### Configuration
- `backlog config get <key>` - Get a config value
- `backlog config set <key> <value>` - Set a config value
- `backlog config list` - List all config values
- `backlog config path` - Show config file path

### Server
- `backlog serve` - Start the OAuth relay server

## Configuration

### User Configuration

Located at `~/.config/backlog/config.yaml`:

```yaml
client:
  default:
    relay_server: "https://relay.example.com"
    space: "mycompany"
    domain: "backlog.jp"
    project: "PROJ"
```

### Project Configuration

Create `.backlog.yaml` in your repository root:

```yaml
project: PROJ
```

## Environment Variables

- `BACKLOG_RELAY_SERVER` - Relay server URL
- `BACKLOG_SPACE` - Default space
- `BACKLOG_DOMAIN` - Default domain
- `BACKLOG_PROJECT` - Default project

## Shell Completion

```bash
# Bash
source <(backlog completion bash)

# Zsh
backlog completion zsh > "${fpath[1]}/_backlog"

# Fish
backlog completion fish | source
```

## License

MIT License
```

## 5. Makefile 更新

```makefile
# リリース用ターゲット追加

.PHONY: release
release: test lint
	@echo "Creating release..."
	goreleaser release --clean

.PHONY: snapshot
snapshot: test
	goreleaser release --snapshot --clean

.PHONY: docs
docs:
	@echo "Generating documentation..."
	go run ./cmd/gendocs
```

## 6. goreleaser 設定

### .goreleaser.yaml

```yaml
project_name: backlog-cli

before:
  hooks:
    - go mod tidy
    - go generate ./...

builds:
  - id: backlog
    main: ./cmd/backlog
    binary: backlog
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X github.com/yourorg/backlog-cli/internal/cmd.Version={{.Version}}
      - -X github.com/yourorg/backlog-cli/internal/cmd.Commit={{.Commit}}
      - -X github.com/yourorg/backlog-cli/internal/cmd.BuildDate={{.Date}}

archives:
  - id: default
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: 'checksums.txt'

changelog:
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
      - '^chore:'

brews:
  - name: backlog-cli
    repository:
      owner: yourorg
      name: homebrew-tap
    homepage: "https://github.com/yourorg/backlog-cli"
    description: "CLI for Backlog project management"
    install: |
      bin.install "backlog"
    test: |
      system "#{bin}/backlog", "version"
```

## 7. GitHub Actions

### .github/workflows/ci.yml

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      
      - name: Build
        run: make build
      
      - name: Test
        run: make test
      
      - name: Lint
        uses: golangci/golangci-lint-action@v4
        with:
          version: latest

  release:
    needs: test
    runs-on: ubuntu-latest
    if: startsWith(github.ref, 'refs/tags/')
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      
      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v5
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

## 完了条件

- [ ] エラーメッセージが統一されている
- [ ] 適切な終了コードが返される
- [ ] シェル補完が動作する（bash, zsh, fish）
- [ ] 主要な機能のテストがある
- [ ] README.md が整備されている
- [ ] `make release` でリリースビルドが作成できる
- [ ] GitHub Actions でCI/CDが動作する

## 完了！

おめでとうございます！Backlog CLI の実装が完了しました。

### 今後の改善案

1. **機能追加**
   - `backlog issue search` - 高度な検索
   - `backlog notification` - 通知管理
   - `backlog activity` - アクティビティフィード
   - `backlog star` - スター管理

2. **UX改善**
   - インタラクティブモード
   - ファジー検索
   - キャッシュ機能

3. **拡張**
   - プラグインシステム
   - カスタムエイリアス
   - フック機能

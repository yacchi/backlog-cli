# Phase 02: 設定管理

## 目標

- 統一設定ファイルの構造定義
- embed によるデフォルト値
- ファイル、環境変数からの読み込み
- プロジェクトローカル設定 (.backlog.yaml) のサポート
- 設定のマージと解決

## 1. 設定ファイル構造

### ~/.config/backlog/config.yaml

```yaml
client:
  default:
    relay_server: ""
    space: ""
    domain: "backlog.jp"
    project: ""
    output: "table"
    color: "auto"
    editor: ""
    browser: ""
  
  auth:
    callback_port: 0
    timeout: 120
    no_browser: false
  
  projects: {}
  
  credentials: {}

server:
  host: "0.0.0.0"
  port: 8080
  base_url: ""
  
  cookie:
    secret: ""
    max_age: 300
  
  backlog: []
  
  access_control:
    allowed_projects: []
    allowed_spaces: []
    allowed_cidrs: []
  
  audit:
    enabled: true
    output: "stdout"
    file_path: ""
    webhook_url: ""
```

## 2. 設定構造体

### internal/config/config.go

```go
package config

import (
	"time"
)

// Config はアプリケーション全体の設定
type Config struct {
	Client ClientConfig `yaml:"client"`
	Server ServerConfig `yaml:"server"`
}

// ClientConfig はCLIクライアントの設定
type ClientConfig struct {
	Default     ClientDefaultConfig            `yaml:"default"`
	Auth        ClientAuthConfig               `yaml:"auth"`
	Projects    map[string]ProjectOverride     `yaml:"projects"`
	Credentials map[string]Credential          `yaml:"credentials"`
}

// ClientDefaultConfig はクライアントのデフォルト設定
type ClientDefaultConfig struct {
	RelayServer string `yaml:"relay_server" env:"BACKLOG_RELAY_SERVER"`
	Space       string `yaml:"space" env:"BACKLOG_SPACE"`
	Domain      string `yaml:"domain" env:"BACKLOG_DOMAIN"`
	Project     string `yaml:"project" env:"BACKLOG_PROJECT"`
	Output      string `yaml:"output" env:"BACKLOG_OUTPUT"`
	Color       string `yaml:"color" env:"BACKLOG_COLOR"`
	Editor      string `yaml:"editor" env:"BACKLOG_EDITOR"`
	Browser     string `yaml:"browser" env:"BACKLOG_BROWSER"`
}

// ClientAuthConfig は認証関連の設定
type ClientAuthConfig struct {
	CallbackPort int           `yaml:"callback_port" env:"BACKLOG_CALLBACK_PORT"`
	Timeout      int           `yaml:"timeout" env:"BACKLOG_AUTH_TIMEOUT"`
	NoBrowser    bool          `yaml:"no_browser" env:"BACKLOG_NO_BROWSER"`
}

// ProjectOverride はプロジェクト固有の設定オーバーライド
type ProjectOverride struct {
	RelayServer string `yaml:"relay_server,omitempty"`
	Space       string `yaml:"space,omitempty"`
	Domain      string `yaml:"domain,omitempty"`
}

// Credential は認証情報
type Credential struct {
	AccessToken  string    `yaml:"access_token"`
	RefreshToken string    `yaml:"refresh_token"`
	ExpiresAt    time.Time `yaml:"expires_at"`
	UserID       string    `yaml:"user_id"`
	UserName     string    `yaml:"user_name"`
}

// ServerConfig は中継サーバーの設定
type ServerConfig struct {
	Host    string             `yaml:"host" env:"BACKLOG_SERVER_HOST"`
	Port    int                `yaml:"port" env:"BACKLOG_SERVER_PORT"`
	BaseURL string             `yaml:"base_url" env:"BACKLOG_SERVER_BASE_URL"`
	Cookie  CookieConfig       `yaml:"cookie"`
	Backlog []BacklogAppConfig `yaml:"backlog"`
	Access  AccessControl      `yaml:"access_control"`
	Audit   AuditConfig        `yaml:"audit"`
}

// CookieConfig はCookie設定
type CookieConfig struct {
	Secret string `yaml:"secret" env:"BACKLOG_COOKIE_SECRET"`
	MaxAge int    `yaml:"max_age"`
}

// BacklogAppConfig はBacklogアプリケーションの設定
type BacklogAppConfig struct {
	Domain       string `yaml:"domain"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

// AccessControl はアクセス制御設定
type AccessControl struct {
	AllowedProjects []string `yaml:"allowed_projects" env:"BACKLOG_ALLOWED_PROJECTS"`
	AllowedSpaces   []string `yaml:"allowed_spaces" env:"BACKLOG_ALLOWED_SPACES"`
	AllowedCIDRs    []string `yaml:"allowed_cidrs" env:"BACKLOG_ALLOWED_CIDRS"`
}

// AuditConfig は監査ログ設定
type AuditConfig struct {
	Enabled    bool   `yaml:"enabled" env:"BACKLOG_AUDIT_ENABLED"`
	Output     string `yaml:"output" env:"BACKLOG_AUDIT_OUTPUT"`
	FilePath   string `yaml:"file_path" env:"BACKLOG_AUDIT_FILE_PATH"`
	WebhookURL string `yaml:"webhook_url" env:"BACKLOG_AUDIT_WEBHOOK_URL"`
}
```

## 3. プロジェクトローカル設定

### internal/config/project.go

```go
package config

// ProjectConfig はプロジェクトローカル設定 (.backlog.yaml)
type ProjectConfig struct {
	Project     string `yaml:"project"`
	Space       string `yaml:"space,omitempty"`
	Domain      string `yaml:"domain,omitempty"`
	RelayServer string `yaml:"relay_server,omitempty"`
}

// ProjectConfigFiles は検索するファイル名の優先順
var ProjectConfigFiles = []string{
	".backlog.yaml",
	".backlog.yml",
	".backlog-project.yaml",
	".backlog-project.yml",
}
```

## 4. デフォルト値 (embed)

### internal/config/defaults.yaml

```yaml
client:
  default:
    domain: "backlog.jp"
    output: "table"
    color: "auto"
  auth:
    callback_port: 0
    timeout: 120
    no_browser: false

server:
  host: "0.0.0.0"
  port: 8080
  cookie:
    max_age: 300
  audit:
    enabled: true
    output: "stdout"
```

### internal/config/defaults.go

```go
package config

import (
	_ "embed"

	"gopkg.in/yaml.v3"
)

//go:embed defaults.yaml
var defaultConfigYAML []byte

// DefaultConfig はデフォルト設定を返す
func DefaultConfig() (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(defaultConfigYAML, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
```

## 5. 設定ローダー

### internal/config/loader.go

```go
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigDir は設定ディレクトリのパスを返す
func ConfigDir() (string, error) {
	// XDG_CONFIG_HOME を優先
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "backlog"), nil
	}
	
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "backlog"), nil
}

// ConfigPath は設定ファイルのパスを返す
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load は設定ファイルを読み込む
func Load() (*Config, error) {
	// 1. デフォルト値を読み込み
	cfg, err := DefaultConfig()
	if err != nil {
		return nil, err
	}
	
	// 2. 設定ファイルを読み込み、マージ
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}
	
	// 3. 環境変数でオーバーライド
	applyEnvOverrides(cfg)
	
	return cfg, nil
}

// LoadFromFile は指定したファイルから設定を読み込む
func LoadFromFile(path string) (*Config, error) {
	cfg, err := DefaultConfig()
	if err != nil {
		return nil, err
	}
	
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	
	applyEnvOverrides(cfg)
	
	return cfg, nil
}

// Save は設定をファイルに保存する
func Save(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	
	// ディレクトリ作成
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, data, 0600)
}
```

## 6. 環境変数オーバーライド

### internal/config/env.go

```go
package config

import (
	"os"
	"strconv"
	"strings"
)

func applyEnvOverrides(cfg *Config) {
	// Client settings
	if v := os.Getenv("BACKLOG_RELAY_SERVER"); v != "" {
		cfg.Client.Default.RelayServer = v
	}
	if v := os.Getenv("BACKLOG_SPACE"); v != "" {
		cfg.Client.Default.Space = v
	}
	if v := os.Getenv("BACKLOG_DOMAIN"); v != "" {
		cfg.Client.Default.Domain = v
	}
	if v := os.Getenv("BACKLOG_PROJECT"); v != "" {
		cfg.Client.Default.Project = v
	}
	if v := os.Getenv("BACKLOG_OUTPUT"); v != "" {
		cfg.Client.Default.Output = v
	}
	if v := os.Getenv("BACKLOG_NO_BROWSER"); v != "" {
		cfg.Client.Auth.NoBrowser = v == "true" || v == "1"
	}
	if v := os.Getenv("BACKLOG_CALLBACK_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Client.Auth.CallbackPort = port
		}
	}
	
	// Server settings
	if v := os.Getenv("BACKLOG_SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv("BACKLOG_COOKIE_SECRET"); v != "" {
		cfg.Server.Cookie.Secret = v
	}
	
	// Backlog JP credentials
	if clientID := os.Getenv("BACKLOG_JP_CLIENT_ID"); clientID != "" {
		clientSecret := os.Getenv("BACKLOG_JP_CLIENT_SECRET")
		cfg.Server.Backlog = upsertBacklogConfig(cfg.Server.Backlog, "backlog.jp", clientID, clientSecret)
	}
	
	// Backlog COM credentials
	if clientID := os.Getenv("BACKLOG_COM_CLIENT_ID"); clientID != "" {
		clientSecret := os.Getenv("BACKLOG_COM_CLIENT_SECRET")
		cfg.Server.Backlog = upsertBacklogConfig(cfg.Server.Backlog, "backlog.com", clientID, clientSecret)
	}
	
	// Access control
	if v := os.Getenv("BACKLOG_ALLOWED_PROJECTS"); v != "" {
		cfg.Server.Access.AllowedProjects = strings.Split(v, ",")
	}
	if v := os.Getenv("BACKLOG_ALLOWED_SPACES"); v != "" {
		cfg.Server.Access.AllowedSpaces = strings.Split(v, ",")
	}
	
	// Audit
	if v := os.Getenv("BACKLOG_AUDIT_WEBHOOK_URL"); v != "" {
		cfg.Server.Audit.WebhookURL = v
	}
}

func upsertBacklogConfig(configs []BacklogAppConfig, domain, clientID, clientSecret string) []BacklogAppConfig {
	for i, c := range configs {
		if c.Domain == domain {
			configs[i].ClientID = clientID
			configs[i].ClientSecret = clientSecret
			return configs
		}
	}
	return append(configs, BacklogAppConfig{
		Domain:       domain,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	})
}
```

## 7. プロジェクトローカル設定の検索

### internal/config/project.go (追加)

```go
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FindProjectConfig はカレントディレクトリから上に向かって
// .backlog.yaml を検索する
func FindProjectConfig() (*ProjectConfig, string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	
	for {
		for _, name := range ProjectConfigFiles {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				cfg, err := loadProjectConfig(path)
				if err != nil {
					return nil, "", err
				}
				return cfg, path, nil
			}
		}
		
		parent := filepath.Dir(dir)
		if parent == dir {
			// ルートに到達、見つからず
			return nil, "", nil
		}
		dir = parent
	}
}

func loadProjectConfig(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	
	return &cfg, nil
}

// CreateProjectConfig はプロジェクト設定ファイルを作成する
func CreateProjectConfig(projectKey string) error {
	cfg := ProjectConfig{
		Project: projectKey,
	}
	
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	
	return os.WriteFile(".backlog.yaml", data, 0644)
}
```

## 8. 設定リゾルバー

### internal/config/resolver.go

```go
package config

// ResolvedConfig は解決済みの設定（実行時に使用）
type ResolvedConfig struct {
	RelayServer string
	Space       string
	Domain      string
	Project     string
	Output      string
	Color       string
	
	// 認証設定
	CallbackPort int
	Timeout      int
	NoBrowser    bool
	
	// 認証情報
	Credential *Credential
}

// ResolveOptions は設定解決時のオプション
type ResolveOptions struct {
	// コマンドライン引数
	Project     string
	Space       string
	Domain      string
	RelayServer string
	Output      string
}

// Resolve は優先順位に従って設定を解決する
func Resolve(cfg *Config, opts ResolveOptions) (*ResolvedConfig, error) {
	resolved := &ResolvedConfig{
		// デフォルト値
		RelayServer:  cfg.Client.Default.RelayServer,
		Space:        cfg.Client.Default.Space,
		Domain:       cfg.Client.Default.Domain,
		Project:      cfg.Client.Default.Project,
		Output:       cfg.Client.Default.Output,
		Color:        cfg.Client.Default.Color,
		CallbackPort: cfg.Client.Auth.CallbackPort,
		Timeout:      cfg.Client.Auth.Timeout,
		NoBrowser:    cfg.Client.Auth.NoBrowser,
	}
	
	// プロジェクトローカル設定 (.backlog.yaml)
	if projectCfg, _, err := FindProjectConfig(); err == nil && projectCfg != nil {
		if projectCfg.Project != "" {
			resolved.Project = projectCfg.Project
		}
		if projectCfg.Space != "" {
			resolved.Space = projectCfg.Space
		}
		if projectCfg.Domain != "" {
			resolved.Domain = projectCfg.Domain
		}
		if projectCfg.RelayServer != "" {
			resolved.RelayServer = projectCfg.RelayServer
		}
	}
	
	// プロジェクト固有設定 (config.yaml内)
	if resolved.Project != "" {
		if override, ok := cfg.Client.Projects[resolved.Project]; ok {
			if override.Space != "" {
				resolved.Space = override.Space
			}
			if override.Domain != "" {
				resolved.Domain = override.Domain
			}
			if override.RelayServer != "" {
				resolved.RelayServer = override.RelayServer
			}
		}
	}
	
	// コマンドライン引数（最優先）
	if opts.Project != "" {
		resolved.Project = opts.Project
	}
	if opts.Space != "" {
		resolved.Space = opts.Space
	}
	if opts.Domain != "" {
		resolved.Domain = opts.Domain
	}
	if opts.RelayServer != "" {
		resolved.RelayServer = opts.RelayServer
	}
	if opts.Output != "" {
		resolved.Output = opts.Output
	}
	
	// 認証情報の解決
	if resolved.Space != "" && resolved.Domain != "" {
		host := resolved.Space + "." + resolved.Domain
		if cred, ok := cfg.Client.Credentials[host]; ok {
			resolved.Credential = &cred
		}
	}
	
	return resolved, nil
}
```

## 9. テスト

### internal/config/config_test.go

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig() error = %v", err)
	}
	
	if cfg.Client.Default.Domain != "backlog.jp" {
		t.Errorf("Domain = %v, want %v", cfg.Client.Default.Domain, "backlog.jp")
	}
	
	if cfg.Client.Default.Output != "table" {
		t.Errorf("Output = %v, want %v", cfg.Client.Default.Output, "table")
	}
}

func TestEnvOverrides(t *testing.T) {
	os.Setenv("BACKLOG_SPACE", "test-space")
	defer os.Unsetenv("BACKLOG_SPACE")
	
	cfg, _ := DefaultConfig()
	applyEnvOverrides(cfg)
	
	if cfg.Client.Default.Space != "test-space" {
		t.Errorf("Space = %v, want %v", cfg.Client.Default.Space, "test-space")
	}
}

func TestFindProjectConfig(t *testing.T) {
	// テスト用ディレクトリ作成
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub", "dir")
	os.MkdirAll(subDir, 0755)
	
	// .backlog.yaml作成
	configPath := filepath.Join(tmpDir, ".backlog.yaml")
	os.WriteFile(configPath, []byte("project: TEST-PROJ\n"), 0644)
	
	// サブディレクトリに移動
	oldDir, _ := os.Getwd()
	os.Chdir(subDir)
	defer os.Chdir(oldDir)
	
	// 検索
	cfg, path, err := FindProjectConfig()
	if err != nil {
		t.Fatalf("FindProjectConfig() error = %v", err)
	}
	
	if cfg == nil {
		t.Fatal("FindProjectConfig() returned nil")
	}
	
	if cfg.Project != "TEST-PROJ" {
		t.Errorf("Project = %v, want %v", cfg.Project, "TEST-PROJ")
	}
	
	if path != configPath {
		t.Errorf("Path = %v, want %v", path, configPath)
	}
}
```

## 完了条件

- [ ] `config.Load()` でデフォルト設定が読み込める
- [ ] 設定ファイルがある場合、マージされる
- [ ] 環境変数でオーバーライドできる
- [ ] `.backlog.yaml` が検索できる
- [ ] `Resolve()` で優先順位に従った設定が取得できる
- [ ] テストが通る

## 次のステップ

`03-relay-server.md` に進んで中継サーバーの基本機能を実装してください。

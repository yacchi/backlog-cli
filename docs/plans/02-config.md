# Phase 02: 設定管理

## 目標

- レイヤードアーキテクチャによる設定管理
- 複数プロファイルのサポート
- embed によるデフォルト値
- ファイル、環境変数からの読み込み
- プロジェクトローカル設定 (.backlog.yaml) のサポート
- 認証情報の別ファイル管理
- 設定のマージと解決

## 1. 設定ファイル構造

### ~/.config/backlog/config.yaml

```yaml
# プロファイル設定（複数定義可能）
profile:
  default:
    relay_server: ""
    space: ""
    domain: "backlog.jp"
    project: ""
    output: "table"
    color: "auto"
    editor: ""
    browser: ""
    auth_callback_port: 0
    auth_timeout: 120
    auth_no_browser: false
    http_timeout: 30
    http_token_refresh_margin: 300

  # 追加プロファイル例
  work:
    relay_server: "https://relay.company.com"
    space: "mycompany"
    domain: "backlog.jp"

# サーバー設定（中継サーバー用）
server:
  host: "0.0.0.0"
  port: 8080
  base_url: ""
  http:
    read_timeout: 10
    write_timeout: 30
    idle_timeout: 60
  cookie:
    secret: ""
    max_age: 300
  jwt:
    expiry: 3600
  rate_limit:
    enabled: false
    requests_per_minute: 60
    burst: 10
    cleanup_interval: 300
    entry_ttl: 600
  audit:
    enabled: false
    output: "stdout"
    file_path: ""
    webhook_url: ""
    webhook_timeout: 10
  access_control:
    allowed_spaces: []
    allowed_projects: []
    allowed_cidrs: []
  backlog:
    - domain: "backlog.jp"
      client_id: ""
      client_secret: ""

# 表示設定
display:
  summary_max_length: 50
  default_comment_count: 10
  default_issue_limit: 20
  timezone: ""
  date_format: "2006-01-02"
  datetime_format: "2006-01-02 15:04"
  hyperlink: true
  issue_list_fields:
    - key
    - status
    - priority
    - assignee
    - summary
  pr_list_fields:
    - number
    - status
    - author
    - branch
    - summary
```

### ~/.config/backlog/credentials/{profile}.yaml

認証情報はプロファイルごとに別ファイルで管理：

```yaml
auth_type: oauth  # oauth または apikey
access_token: "..."
refresh_token: "..."
expires_at: "2025-01-01T00:00:00Z"
user_id: "123"
user_name: "John Doe"

# API Key認証の場合
# auth_type: apikey
# api_key: "..."
```

## 2. レイヤードアーキテクチャ

設定は以下の優先順位でマージされる（低→高）：

1. **LayerDefaults** - 内蔵デフォルト値（embed）
2. **LayerUser** - ~/.config/backlog/config.yaml
3. **LayerProject** - .backlog.yaml（カレントディレクトリから上位を検索）
4. **LayerEnv** - 環境変数（BACKLOG_*）
5. **LayerCredentials** - ~/.config/backlog/credentials/{profile}.yaml
6. **LayerArgs** - コマンドライン引数（最優先）

## 3. 設定構造体

### internal/config/config.go

```go
package config

import "time"

// DefaultProfile はデフォルトのプロファイル名
const DefaultProfile = "default"

// ConfigKey は設定のキー名
type ConfigKey string

// 設定キー定数
const (
	KeyRelayServer            ConfigKey = "profile.default.relay_server"
	KeySpace                  ConfigKey = "profile.default.space"
	KeyDomain                 ConfigKey = "profile.default.domain"
	KeyProject                ConfigKey = "profile.default.project"
	KeyOutput                 ConfigKey = "profile.default.output"
	KeyColor                  ConfigKey = "profile.default.color"
	KeyEditor                 ConfigKey = "profile.default.editor"
	KeyBrowser                ConfigKey = "profile.default.browser"
	KeyAuthCallbackPort       ConfigKey = "profile.default.auth_callback_port"
	KeyAuthTimeout            ConfigKey = "profile.default.auth_timeout"
	KeyAuthNoBrowser          ConfigKey = "profile.default.auth_no_browser"
	KeyHTTPTimeout            ConfigKey = "profile.default.http_timeout"
	KeyHTTPTokenRefreshMargin ConfigKey = "profile.default.http_token_refresh_margin"
	// ... 他のキー
)

// 環境変数キー定数
const (
	EnvBacklogProfile     = "BACKLOG_PROFILE"
	EnvBacklogRelayServer = "BACKLOG_RELAY_SERVER"
	EnvBacklogSpace       = "BACKLOG_SPACE"
	EnvBacklogDomain      = "BACKLOG_DOMAIN"
	EnvBacklogProject     = "BACKLOG_PROJECT"
	// ... 他の環境変数
)

// Profile はプロファイル設定
type Profile struct {
	RelayServer            string `yaml:"relay_server,omitempty"`
	Space                  string `yaml:"space,omitempty"`
	Domain                 string `yaml:"domain,omitempty"`
	Project                string `yaml:"project,omitempty"`
	Output                 string `yaml:"output,omitempty"`
	Color                  string `yaml:"color,omitempty"`
	Editor                 string `yaml:"editor,omitempty"`
	Browser                string `yaml:"browser,omitempty"`
	AuthCallbackPort       int    `yaml:"auth_callback_port,omitempty"`
	AuthTimeout            int    `yaml:"auth_timeout,omitempty"`
	AuthNoBrowser          bool   `yaml:"auth_no_browser,omitempty"`
	HTTPTimeout            int    `yaml:"http_timeout,omitempty"`
	HTTPTokenRefreshMargin int    `yaml:"http_token_refresh_margin,omitempty"`
}

// AuthType は認証タイプ
type AuthType string

const (
	AuthTypeOAuth  AuthType = "oauth"
	AuthTypeAPIKey AuthType = "apikey"
)

// Credential は認証情報（別ファイル管理）
type Credential struct {
	AuthType     AuthType  `yaml:"auth_type,omitempty"`
	AccessToken  string    `yaml:"access_token,omitempty"`
	RefreshToken string    `yaml:"refresh_token,omitempty"`
	ExpiresAt    time.Time `yaml:"expires_at,omitempty"`
	APIKey       string    `yaml:"api_key,omitempty"`
	UserID       string    `yaml:"user_id,omitempty"`
	UserName     string    `yaml:"user_name,omitempty"`
}

// BacklogAppConfig はBacklogアプリケーション設定
type BacklogAppConfig struct {
	domain       string
	clientID     string
	clientSecret string
}

// FieldConfig はテーブル表示時のフィールド設定
type FieldConfig struct {
	header     string
	maxWidth   int
	timeFormat string
}
```

### internal/config/store.go

```go
package config

// LayerName はレイヤーの名前
type LayerName string

const (
	LayerDefaults    LayerName = "defaults"
	LayerUser        LayerName = "user"
	LayerProject     LayerName = "project"
	LayerEnv         LayerName = "env"
	LayerCredentials LayerName = "credentials"
	LayerArgs        LayerName = "args"
)

// Config は設定のレイヤー管理を行う
type Config struct {
	layers            map[LayerName]map[string]any
	layerOrder        []LayerName
	data              *configData
	projectConfigPath string
	activeProfile     string
}

// NewConfig は新しいConfigを作成する
func NewConfig() *Config {
	return &Config{
		layers: map[LayerName]map[string]any{
			LayerDefaults:    make(map[string]any),
			LayerUser:        make(map[string]any),
			LayerProject:     make(map[string]any),
			LayerEnv:         make(map[string]any),
			LayerCredentials: make(map[string]any),
			LayerArgs:        make(map[string]any),
		},
		layerOrder:    []LayerName{LayerDefaults, LayerUser, LayerProject, LayerEnv, LayerCredentials, LayerArgs},
		activeProfile: DefaultProfile,
	}
}

// LoadAll は全レイヤーを読み込む
func (s *Config) LoadAll() error {
	// 1. デフォルト値（embed）
	// 2. ユーザー設定（~/.config/backlog/config.yaml）
	// 3. プロジェクト設定（.backlog.yaml）
	// 4. 環境変数
	// 5. クレデンシャル（別ファイル）
	// 6. 設定をマージしてmaterialize
	return nil
}
```

## 4. プロジェクトローカル設定

### internal/config/project.go

```go
package config

// ProjectSetting はプロジェクト設定
type ProjectSetting struct {
	profile string // 使用するプロファイル名
	name    string // プロジェクトキー
}

// ProjectConfigFiles は検索するファイル名の優先順
var ProjectConfigFiles = []string{
	".backlog.yaml",
	".backlog.yml",
}
```

## 5. デフォルト値 (embed)

### internal/config/defaults.yaml

```yaml
profile:
  default:
    domain: "backlog.jp"
    output: "table"
    color: "auto"
    auth_callback_port: 0
    auth_timeout: 120
    auth_no_browser: false
    http_timeout: 30
    http_token_refresh_margin: 300

server:
  host: "0.0.0.0"
  port: 8080
  http:
    read_timeout: 10
    write_timeout: 30
    idle_timeout: 60
  cookie:
    max_age: 300
  jwt:
    expiry: 3600
  rate_limit:
    enabled: false
    requests_per_minute: 60
    burst: 10
    cleanup_interval: 300
    entry_ttl: 600
  audit:
    enabled: false
    output: "stdout"
    webhook_timeout: 10

display:
  summary_max_length: 50
  default_comment_count: 10
  default_issue_limit: 20
  date_format: "2006-01-02"
  datetime_format: "2006-01-02 15:04"
  hyperlink: true
  issue_list_fields:
    - key
    - status
    - priority
    - assignee
    - summary
  pr_list_fields:
    - number
    - status
    - author
    - branch
    - summary

auth:
  min_callback_port: 49152
  max_callback_port: 65535
```

### internal/config/defaults.go

```go
package config

import (
	_ "embed"
)

//go:embed defaults.yaml
var defaultConfigYAML []byte
```

## 6. 設定ローダー

### internal/config/loader.go

```go
package config

import (
	"os"
	"path/filepath"
)

// ConfigDir は設定ディレクトリのパスを返す
func ConfigDir() (string, error) {
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

// CredentialsDir はクレデンシャルディレクトリのパスを返す
func CredentialsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials"), nil
}

// findProjectConfigPath はプロジェクト設定ファイルを検索する
func findProjectConfigPath() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		for _, name := range ProjectConfigFiles {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil // 見つからず
		}
		dir = parent
	}
}
```

## 7. 設定リゾルバー

### internal/config/resolver.go

```go
package config

// ResolvedConfig は解決済みの設定（実行時に使用）
type ResolvedConfig struct {
	Profile     string
	RelayServer string
	Space       string
	Domain      string
	Project     string
	Output      string
	Color       string
	Editor      string
	Browser     string

	// 認証設定
	CallbackPort int
	Timeout      int
	NoBrowser    bool

	// HTTP設定
	HTTPTimeout            int
	HTTPTokenRefreshMargin int

	// 認証情報
	Credential *Credential
}

// CLIFlags はCLIから渡されるフラグ
type CLIFlags struct {
	Profile string
	Project string
	Space   string
	Domain  string
	Output  string
	Color   string
}

// CreateResolver はResolverを作成する
func (s *Config) CreateResolver(flags CLIFlags) (*ResolvedConfig, error) {
	// アクティブプロファイルを決定
	profileName := s.activeProfile
	if flags.Profile != "" {
		profileName = flags.Profile
	}

	// フラグをArgsレイヤーに設定
	if flags.Project != "" {
		s.Set(LayerArgs, KeyProjectName.String(), flags.Project)
	}
	// ...他のフラグも同様

	// 設定を再マテリアライズ
	s.materialize()

	// ResolvedConfigを構築
	profile := s.GetProfile(profileName)
	return &ResolvedConfig{
		Profile:                profileName,
		RelayServer:            profile.RelayServer,
		Space:                  profile.Space,
		Domain:                 profile.Domain,
		Project:                s.data.Project.Name(),
		Output:                 profile.Output,
		Color:                  profile.Color,
		Editor:                 profile.Editor,
		Browser:                profile.Browser,
		CallbackPort:           profile.AuthCallbackPort,
		Timeout:                profile.AuthTimeout,
		NoBrowser:              profile.AuthNoBrowser,
		HTTPTimeout:            profile.HTTPTimeout,
		HTTPTokenRefreshMargin: profile.HTTPTokenRefreshMargin,
		Credential:             s.GetCredential(profileName),
	}, nil
}
```

## 8. クレデンシャル管理

### internal/config/credentials.go

```go
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// loadCredentials はクレデンシャルファイルを読み込む
func (s *Config) loadCredentials() error {
	dir, err := CredentialsDir()
	if err != nil {
		return err
	}

	// credentials/{profile}.yaml を読み込む
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		profileName := name[:len(name)-len(ext)]
		path := filepath.Join(dir, name)

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var cred Credential
		if err := yaml.Unmarshal(data, &cred); err != nil {
			continue
		}

		s.data.Credentials[profileName] = &cred
	}

	return nil
}

// SaveCredentials はクレデンシャルを保存する
func (s *Config) SaveCredentials() error {
	dir, err := CredentialsDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	for profileName, cred := range s.data.Credentials {
		path := filepath.Join(dir, profileName+".yaml")
		data, err := yaml.Marshal(cred)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			return err
		}
	}

	return nil
}

// SetCredential はクレデンシャルを設定する
func (s *Config) SetCredential(profileName string, cred Credential) {
	s.data.Credentials[profileName] = &cred
}

// GetCredential はクレデンシャルを取得する
func (s *Config) GetCredential(profileName string) *Credential {
	return s.data.Credentials[profileName]
}
```

## 9. アクセサー

### internal/config/accessor.go

```go
package config

// GetProfile はプロファイルを取得する
func (s *Config) GetProfile(name string) *Profile {
	if profile, ok := s.data.Profiles[name]; ok {
		return profile
	}
	// デフォルトプロファイルを返す
	return s.data.Profiles[DefaultProfile]
}

// GetBacklogApp はドメインに対応するBacklog設定を取得する
func (s *Config) GetBacklogApp(domain string) *BacklogAppConfig {
	for _, app := range s.data.ServerBacklog {
		if app.Domain() == domain {
			return &app
		}
	}
	return nil
}

// ServerPort はサーバーポートを返す
func (s *Config) ServerPort() int {
	return s.data.ServerPort
}

// ServerHost はサーバーホストを返す
func (s *Config) ServerHost() string {
	return s.data.ServerHost
}

// CookieSecret はCookie署名シークレットを返す
func (s *Config) CookieSecret() string {
	return s.data.ServerCookieSecret
}

// ... 他のアクセサー
```

## 10. 完了条件

- [x] レイヤードアーキテクチャによる設定管理
- [x] 複数プロファイルのサポート
- [x] embed によるデフォルト値
- [x] ファイル、環境変数からの読み込み
- [x] プロジェクトローカル設定のサポート
- [x] 認証情報の別ファイル管理
- [x] 設定のマージと解決

## 次のステップ

`03-relay-server.md` に進んで中継サーバーの基本機能を実装してください。

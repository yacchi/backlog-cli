package config

import (
	"os"
	"time"
)

// ResolvedConfig は全レイヤーをマージし、デフォルト適用後の設定
// 全フィールドは具体的な値を持つ
// jubakoのmaterializationはJSONを使用するため、jsonタグが必須
// jubako tagでYAMLパスからマッピング（ネスト構造を解決）
type ResolvedConfig struct {
	// アクティブプロファイル名 (runtime only, not from config)
	ActiveProfile string `json:"active_profile" jubako:"-"`

	// プロファイル設定（キーはプロファイル名）
	Profiles map[string]*ResolvedProfile `json:"profile" jubako:"/profile"`

	// クレデンシャル設定（キーはプロファイル名）
	// sensitive タグにより、このフィールドとその子はセンシティブレイヤーにのみ書き込み可能
	Credentials map[string]*Credential `json:"credential" jubako:"/credential"`

	// プロジェクト設定
	Project ResolvedProject `json:"project" jubako:"/project"`

	// サーバー設定
	Server ResolvedServer `json:"server"`

	// 表示設定
	Display ResolvedDisplay `json:"display"`

	// 認証設定
	Auth ResolvedAuth `json:"auth"`

	// キャッシュ設定
	Cache ResolvedCache `json:"cache"`
}

// ResolvedCache はマージ済みのキャッシュ設定
// jubako tagでcache.*からマッピング
// env: ディレクティブで環境変数からの自動マッピングを定義
type ResolvedCache struct {
	Enabled bool   `json:"enabled" jubako:"/cache/enabled,env:CACHE_ENABLED"`
	Dir     string `json:"dir" jubako:"/cache/dir,env:CACHE_DIR"`
	TTL     int    `json:"ttl" jubako:"/cache/ttl,env:CACHE_TTL"`
}

// GetCacheDir returns the cache directory.
// If Dir is not specified, it returns the default cache directory.
func (c *ResolvedCache) GetCacheDir() (string, error) {
	if c.Dir != "" {
		return c.Dir, nil
	}
	return defaultCacheDir()
}

// envShortcuts はプロファイル設定の環境変数ショートカットマッピング
// BACKLOG_SPACE などの省略形を BACKLOG_PROFILE_default_SPACE の完全形式に展開する
var envShortcuts = map[string]string{
	"BACKLOG_RELAY_SERVER":      "BACKLOG_PROFILE_default_RELAY_SERVER",
	"BACKLOG_SPACE":             "BACKLOG_PROFILE_default_SPACE",
	"BACKLOG_DOMAIN":            "BACKLOG_PROFILE_default_DOMAIN",
	"BACKLOG_PROJECT":           "BACKLOG_PROFILE_default_PROJECT",
	"BACKLOG_OUTPUT":            "BACKLOG_PROFILE_default_OUTPUT",
	"BACKLOG_COLOR":             "BACKLOG_PROFILE_default_COLOR",
	"BACKLOG_EDITOR":            "BACKLOG_PROFILE_default_EDITOR",
	"BACKLOG_BROWSER":           "BACKLOG_PROFILE_default_BROWSER",
	"BACKLOG_CALLBACK_PORT":     "BACKLOG_PROFILE_default_CALLBACK_PORT",
	"BACKLOG_AUTH_TIMEOUT":      "BACKLOG_PROFILE_default_AUTH_TIMEOUT",
	"BACKLOG_NO_BROWSER":        "BACKLOG_PROFILE_default_NO_BROWSER",
	"BACKLOG_SKIP_CONFIRMATION": "BACKLOG_PROFILE_default_SKIP_CONFIRMATION",
}

// expandEnvShortcuts は環境変数のショートカットを展開した環境変数リストを返す
// os.Environ() の結果に、ショートカット環境変数の展開を追加する
// 完全形式が既に設定されている場合は、完全形式を優先する
func expandEnvShortcuts() []string {
	envs := os.Environ()

	for shortKey, fullKey := range envShortcuts {
		if value := os.Getenv(shortKey); value != "" {
			// 完全形式がまだ設定されていなければ追加
			if os.Getenv(fullKey) == "" {
				envs = append(envs, fullKey+"="+value)
			}
		}
	}

	return envs
}

// ResolvedProfile はマージ済みの非オプショナルなプロファイル設定
// env: ディレクティブで環境変数からの自動マッピングを定義
// {key} プレースホルダーを使用してプロファイル名を動的にマッピング
// 例: BACKLOG_PROFILE_default_SPACE → /profile/default/space
//
// ショートカット環境変数（BACKLOG_SPACE など）は expandEnvShortcuts で
// 完全形式に展開されてからマッピングされる
type ResolvedProfile struct {
	RelayServer            string `json:"relay_server" jubako:",env:PROFILE_{key}_RELAY_SERVER"`
	Space                  string `json:"space" jubako:",env:PROFILE_{key}_SPACE"`
	Domain                 string `json:"domain" jubako:",env:PROFILE_{key}_DOMAIN"`
	Project                string `json:"project" jubako:",env:PROFILE_{key}_PROJECT"`
	Output                 string `json:"output" jubako:",env:PROFILE_{key}_OUTPUT"`
	Format                 string `json:"format" jubako:",env:PROFILE_{key}_FORMAT"`
	Color                  string `json:"color" jubako:",env:PROFILE_{key}_COLOR"`
	Editor                 string `json:"editor" jubako:",env:PROFILE_{key}_EDITOR"`
	Browser                string `json:"browser" jubako:",env:PROFILE_{key}_BROWSER"`
	AuthCallbackPort       int    `json:"auth_callback_port" jubako:",env:PROFILE_{key}_CALLBACK_PORT"`
	AuthTimeout            int    `json:"auth_timeout" jubako:",env:PROFILE_{key}_AUTH_TIMEOUT"`
	AuthNoBrowser          bool   `json:"auth_no_browser" jubako:",env:PROFILE_{key}_NO_BROWSER"`
	AuthSkipConfirmation   bool   `json:"auth_skip_confirmation" jubako:",env:PROFILE_{key}_SKIP_CONFIRMATION"`
	HTTPTimeout            int    `json:"http_timeout" jubako:",env:PROFILE_{key}_HTTP_TIMEOUT"`
	HTTPTokenRefreshMargin int    `json:"http_token_refresh_margin" jubako:",env:PROFILE_{key}_HTTP_TOKEN_REFRESH_MARGIN"`
}

// ResolvedProject はマージ済みのプロジェクト設定
// jubako tagでproject.*からマッピング
// env: ディレクティブで環境変数からの自動マッピングを定義
// BACKLOG_PROFILE は BACKLOG_ prefix + PROFILE でアクティブプロファイルを指定
//
// space/domainはプロジェクト設定で上書き可能。設定されている場合は
// プロファイル設定より優先される。これによりチームメンバー間で
// .backlog.yamlを共有し、個別のグローバル設定なしで利用できる。
type ResolvedProject struct {
	Profile string `json:"profile" jubako:"/project/profile,env:PROFILE"`
	Space   string `json:"space" jubako:"/project/space,env:PROJECT_SPACE"`
	Domain  string `json:"domain" jubako:"/project/domain,env:PROJECT_DOMAIN"`
	Name    string `json:"name" jubako:"/project/name,env:PROJECT_NAME"`
}

// ResolvedServer はマージ済みのサーバー設定
// jubako tagでYAMLのネスト構造からフラットな構造へマッピング
// env: ディレクティブで環境変数からの自動マッピングを定義
type ResolvedServer struct {
	Host    string `json:"host" jubako:"/server/host,env:SERVER_HOST"`
	Port    int    `json:"port" jubako:"/server/port,env:SERVER_PORT"`
	BaseURL string `json:"base_url" jubako:"/server/base_url,env:SERVER_BASE_URL"`

	// 許可するホストパターン（セミコロン区切り、ワイルドカード対応）
	// BaseURL未設定時のHostヘッダー検証に使用
	// 例: "*.lambda-url.*.on.aws;*.run.app"
	AllowedHostPatterns string `json:"allowed_host_patterns" jubako:"/server/allowed_host_patterns,env:ALLOWED_HOST_PATTERNS"`

	// HTTP設定 (server.http.*)
	HTTPReadTimeout  int `json:"http_read_timeout" jubako:"/server/http/read_timeout,env:HTTP_READ_TIMEOUT"`
	HTTPWriteTimeout int `json:"http_write_timeout" jubako:"/server/http/write_timeout,env:HTTP_WRITE_TIMEOUT"`
	HTTPIdleTimeout  int `json:"http_idle_timeout" jubako:"/server/http/idle_timeout,env:HTTP_IDLE_TIMEOUT"`

	// JWT設定 (server.jwt.*)
	JWTExpiry int `json:"jwt_expiry" jubako:"/server/jwt/expiry,env:JWT_EXPIRY"`

	// Backlogアプリ設定 (キーは識別子: jp, com)
	// 動的キーのため環境変数マッピングは手動で行う
	Backlog map[string]ResolvedBacklogApp `json:"backlog" jubako:"/server/backlog"`

	// アクセス制御 (server.access_control.*)
	AllowedSpaces   []string `json:"allowed_spaces" jubako:"/server/access_control/allowed_spaces,env:ALLOWED_SPACES"`
	AllowedProjects []string `json:"allowed_projects" jubako:"/server/access_control/allowed_projects,env:ALLOWED_PROJECTS"`
	AllowedCIDRs    []string `json:"allowed_cidrs" jubako:"/server/access_control/allowed_cidrs,env:ALLOWED_CIDRS"`

	// レートリミット (server.rate_limit.*)
	RateLimitEnabled           bool `json:"rate_limit_enabled" jubako:"/server/rate_limit/enabled,env:RATE_LIMIT_ENABLED"`
	RateLimitRequestsPerMinute int  `json:"rate_limit_requests_per_minute" jubako:"/server/rate_limit/requests_per_minute,env:RATE_LIMIT_REQUESTS_PER_MINUTE"`
	RateLimitBurst             int  `json:"rate_limit_burst" jubako:"/server/rate_limit/burst,env:RATE_LIMIT_BURST"`
	RateLimitCleanupInterval   int  `json:"rate_limit_cleanup_interval" jubako:"/server/rate_limit/cleanup_interval"`
	RateLimitEntryTTL          int  `json:"rate_limit_entry_ttl" jubako:"/server/rate_limit/entry_ttl"`

	// 監査ログ (server.audit.*)
	AuditEnabled        bool   `json:"audit_enabled" jubako:"/server/audit/enabled,env:AUDIT_ENABLED"`
	AuditOutput         string `json:"audit_output" jubako:"/server/audit/output,env:AUDIT_OUTPUT"`
	AuditFilePath       string `json:"audit_file_path" jubako:"/server/audit/file_path,env:AUDIT_FILE_PATH"`
	AuditWebhookURL     string `json:"audit_webhook_url" jubako:"/server/audit/webhook_url,env:AUDIT_WEBHOOK_URL"`
	AuditWebhookTimeout int    `json:"audit_webhook_timeout" jubako:"/server/audit/webhook_timeout,env:AUDIT_WEBHOOK_TIMEOUT"`
}

// ResolvedBacklogApp はマージ済みのBacklogアプリ設定
type ResolvedBacklogApp struct {
	DomainValue       string `json:"domain" jubako:"domain,env:DOMAIN_{key|lower}"`
	ClientIDValue     string `json:"client_id" jubako:"client_id,env:CLIENT_ID_{key|lower}"`
	ClientSecretValue string `json:"client_secret" jubako:"client_secret,env:CLIENT_SECRET_{key|lower},sensitive"`
}

// Domain returns the domain value
func (b *ResolvedBacklogApp) Domain() string {
	return b.DomainValue
}

// ClientID returns the client ID value
func (b *ResolvedBacklogApp) ClientID() string {
	return b.ClientIDValue
}

// ClientSecret returns the client secret value
func (b *ResolvedBacklogApp) ClientSecret() string {
	return b.ClientSecretValue
}

// ResolvedDisplay はマージ済みの表示設定
// jubako tagでdisplay.*からマッピング
// env: ディレクティブで環境変数からの自動マッピングを定義
type ResolvedDisplay struct {
	SummaryMaxLength     int                            `json:"summary_max_length" jubako:"/display/summary_max_length,env:DISPLAY_SUMMARY_MAX_LENGTH"`
	SummaryCommentCount  int                            `json:"summary_comment_count" jubako:"/display/summary_comment_count,env:DISPLAY_SUMMARY_COMMENT_COUNT"`
	DefaultCommentCount  int                            `json:"default_comment_count" jubako:"/display/default_comment_count,env:DISPLAY_DEFAULT_COMMENT_COUNT"`
	DefaultIssueLimit    int                            `json:"default_issue_limit" jubako:"/display/default_issue_limit,env:DISPLAY_DEFAULT_ISSUE_LIMIT"`
	Timezone             string                         `json:"timezone" jubako:"/display/timezone,env:DISPLAY_TIMEZONE"`
	DateFormat           string                         `json:"date_format" jubako:"/display/date_format,env:DISPLAY_DATE_FORMAT"`
	DateTimeFormat       string                         `json:"datetime_format" jubako:"/display/datetime_format,env:DISPLAY_DATETIME_FORMAT"`
	Hyperlink            bool                           `json:"hyperlink" jubako:"/display/hyperlink,env:DISPLAY_HYPERLINK"`
	MarkdownView         bool                           `json:"markdown_view" jubako:"/display/markdown_view,env:DISPLAY_MARKDOWN_VIEW"`
	MarkdownWarn         bool                           `json:"markdown_warn" jubako:"/display/markdown_warn,env:DISPLAY_MARKDOWN_WARN"`
	MarkdownCache        bool                           `json:"markdown_cache" jubako:"/display/markdown_cache,env:DISPLAY_MARKDOWN_CACHE"`
	MarkdownCacheRaw     bool                           `json:"markdown_cache_raw" jubako:"/display/markdown_cache_raw,env:DISPLAY_MARKDOWN_CACHE_RAW"`
	MarkdownCacheExcerpt int                            `json:"markdown_cache_excerpt" jubako:"/display/markdown_cache_excerpt,env:DISPLAY_MARKDOWN_CACHE_EXCERPT"`
	IssueListFields      []string                       `json:"issue_list_fields" jubako:"/display/issue_list_fields"`
	IssueFieldConfig     map[string]ResolvedFieldConfig `json:"issue_field_config" jubako:"/display/issue_field_config"`
	PRListFields         []string                       `json:"pr_list_fields" jubako:"/display/pr_list_fields"`
	PRFieldConfig        map[string]ResolvedFieldConfig `json:"pr_field_config" jubako:"/display/pr_field_config"`
}

// ResolvedFieldConfig はマージ済みのフィールド設定
type ResolvedFieldConfig struct {
	Header     string `json:"header"`
	MaxWidth   int    `json:"max_width"`
	TimeFormat string `json:"time_format"`
}

// ResolvedAuth はマージ済みの認証設定
// jubako tagでauth.*からマッピング
// env: ディレクティブで環境変数からの自動マッピングを定義
type ResolvedAuth struct {
	MinCallbackPort int                   `json:"min_callback_port" jubako:"/auth/min_callback_port,env:AUTH_MIN_CALLBACK_PORT"`
	MaxCallbackPort int                   `json:"max_callback_port" jubako:"/auth/max_callback_port,env:AUTH_MAX_CALLBACK_PORT"`
	Session         ResolvedAuthSession   `json:"session" jubako:"/auth/session"`
	Keepalive       ResolvedAuthKeepalive `json:"keepalive" jubako:"/auth/keepalive"`
}

// ResolvedAuthSession はセッション設定
type ResolvedAuthSession struct {
	CheckInterval int `json:"check_interval" jubako:"/auth/session/check_interval"`
	Timeout       int `json:"timeout" jubako:"/auth/session/timeout"`
}

// CheckIntervalDuration はチェック間隔をtime.Durationで返す
func (s *ResolvedAuthSession) CheckIntervalDuration() time.Duration {
	return time.Duration(s.CheckInterval) * time.Second
}

// TimeoutDuration はタイムアウトをtime.Durationで返す
func (s *ResolvedAuthSession) TimeoutDuration() time.Duration {
	return time.Duration(s.Timeout) * time.Second
}

// ResolvedAuthKeepalive はKeepalive設定
type ResolvedAuthKeepalive struct {
	Interval       int `json:"interval" jubako:"/auth/keepalive/interval"`
	Timeout        int `json:"timeout" jubako:"/auth/keepalive/timeout"`
	ConnectTimeout int `json:"connect_timeout" jubako:"/auth/keepalive/connect_timeout"`
	GracePeriod    int `json:"grace_period" jubako:"/auth/keepalive/grace_period"`
}

// IntervalDuration はkeepalive間隔をtime.Durationで返す
func (k *ResolvedAuthKeepalive) IntervalDuration() time.Duration {
	return time.Duration(k.Interval) * time.Second
}

// TimeoutDuration はタイムアウトをtime.Durationで返す
func (k *ResolvedAuthKeepalive) TimeoutDuration() time.Duration {
	return time.Duration(k.Timeout) * time.Second
}

// ConnectTimeoutDuration は接続タイムアウトをtime.Durationで返す
func (k *ResolvedAuthKeepalive) ConnectTimeoutDuration() time.Duration {
	return time.Duration(k.ConnectTimeout) * time.Second
}

// GracePeriodDuration は切断猶予期間をtime.Durationで返す
func (k *ResolvedAuthKeepalive) GracePeriodDuration() time.Duration {
	return time.Duration(k.GracePeriod) * time.Second
}

// NewResolvedConfig は空のResolvedConfigを作成する
func NewResolvedConfig() *ResolvedConfig {
	return &ResolvedConfig{
		ActiveProfile: DefaultProfile,
		Profiles:      make(map[string]*ResolvedProfile),
		Credentials:   make(map[string]*Credential),
		Display: ResolvedDisplay{
			IssueFieldConfig: make(map[string]ResolvedFieldConfig),
			PRFieldConfig:    make(map[string]ResolvedFieldConfig),
		},
	}
}

// GetProfile は指定プロファイルを取得する
// 存在しない場合はdefaultを返す
func (r *ResolvedConfig) GetProfile(name string) *ResolvedProfile {
	if name == "" {
		name = DefaultProfile
	}
	if p, ok := r.Profiles[name]; ok {
		return p
	}
	return r.Profiles[DefaultProfile]
}

// GetCredential は指定プロファイルのクレデンシャルを取得する
func (r *ResolvedConfig) GetCredential(profileName string) *Credential {
	if profileName == "" {
		profileName = DefaultProfile
	}
	return r.Credentials[profileName]
}

// GetActiveProfile はアクティブプロファイルを取得する
func (r *ResolvedConfig) GetActiveProfile() *ResolvedProfile {
	return r.GetProfile(r.ActiveProfile)
}

// GetActiveCredential はアクティブプロファイルのクレデンシャルを取得する
func (r *ResolvedConfig) GetActiveCredential() *Credential {
	return r.GetCredential(r.ActiveProfile)
}

// GetBacklogApp は指定ドメインのBacklogアプリ設定を取得する
func (r *ResolvedConfig) GetBacklogApp(domain string) *ResolvedBacklogApp {
	for key := range r.Server.Backlog {
		app := r.Server.Backlog[key]
		if app.DomainValue == domain {
			return &app
		}
	}
	return nil
}

// Credential の後方互換メソッド
// IsExpired はトークンの有効期限が切れているか確認する
func (c *Credential) IsExpired() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(c.ExpiresAt)
}

// NeedsRefresh はトークンの更新が必要か確認する（マージン込み）
func (c *Credential) NeedsRefresh(marginSeconds int) bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().Add(time.Duration(marginSeconds) * time.Second).After(c.ExpiresAt)
}

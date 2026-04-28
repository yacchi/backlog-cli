package config

import (
	"fmt"
	"strings"
	"time"
)

// DefaultProfile はデフォルトのプロファイル名
const DefaultProfile = "default"

// AuthType は認証タイプ
type AuthType string

const (
	// AuthTypeOAuth はOAuth 2.0認証
	AuthTypeOAuth AuthType = "oauth"
	// AuthTypeAPIKey はAPI Key認証
	AuthTypeAPIKey AuthType = "apikey"
)

// CredentialBackend は認証情報の保存先バックエンド
type CredentialBackend string

const (
	CredentialBackendAuto    CredentialBackend = "auto"
	CredentialBackendKeyring CredentialBackend = "keyring"
	CredentialBackendFile    CredentialBackend = "file"
)

// NormalizeCredentialBackend は保存先バックエンドを正規化する
func NormalizeCredentialBackend(value string) (CredentialBackend, error) {
	switch CredentialBackend(strings.ToLower(strings.TrimSpace(value))) {
	case "", CredentialBackendAuto:
		return CredentialBackendAuto, nil
	case CredentialBackendKeyring:
		return CredentialBackendKeyring, nil
	case CredentialBackendFile:
		return CredentialBackendFile, nil
	default:
		return "", fmt.Errorf("unsupported credential backend: %q", value)
	}
}

// Credential は認証情報
// 親の Credentials フィールドが jubako:"sensitive" なので、
// センシティブでないフィールドは !sensitive でオプトアウトする
type Credential struct {
	// AuthType は認証タイプ（oauth / apikey）
	// 空の場合は後方互換性のためoauthとして扱う
	AuthType AuthType `yaml:"auth_type,omitempty" json:"auth_type,omitempty"`

	// OAuth認証用（センシティブ - 親から継承）
	AccessToken  string    `yaml:"access_token,omitempty" json:"access_token,omitempty" jubako:"sensitive" storage:"keyring"`
	RefreshToken string    `yaml:"refresh_token,omitempty" json:"refresh_token,omitempty" jubako:"sensitive" storage:"keyring"`
	ExpiresAt    time.Time `yaml:"expires_at,omitempty" json:"expires_at,omitempty"`

	// API Key認証用（センシティブ - 親から継承）
	APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty" jubako:"sensitive" storage:"keyring"`

	// 共通（センシティブでない）
	UserID    string `yaml:"user_id,omitempty" json:"user_id,omitempty"`
	UserName  string `yaml:"user_name,omitempty" json:"user_name,omitempty"`
	UserEmail string `yaml:"user_email,omitempty" json:"user_email,omitempty"`
	Space     string `yaml:"space,omitempty" json:"space,omitempty"`
	Domain    string `yaml:"domain,omitempty" json:"domain,omitempty"`
}

// GetAuthType は認証タイプを返す（後方互換性対応）
func (c *Credential) GetAuthType() AuthType {
	if c.AuthType == "" {
		// 後方互換性: AuthTypeが未設定の場合
		if c.APIKey != "" {
			return AuthTypeAPIKey
		}
		return AuthTypeOAuth
	}
	return c.AuthType
}

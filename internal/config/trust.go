package config

// ResolvedClient はマージ済みのクライアント設定
type ResolvedClient struct {
	Trust ResolvedClientTrust `json:"trust" jubako:"/client/trust"`
}

// ResolvedClientTrust は信頼設定
type ResolvedClientTrust struct {
	Bundles []TrustedBundle `json:"bundles" jubako:"/client/trust/bundles"`
}

// TrustedBundle はインポート済みのバンドル設定
type TrustedBundle struct {
	ID            string            `json:"id" yaml:"id"`
	RelayURL      string            `json:"relay_url" yaml:"relay_url"`
	AllowedDomain string            `json:"allowed_domain" yaml:"allowed_domain"`
	BundleToken   string            `json:"bundle_token,omitempty" yaml:"bundle_token,omitempty"`
	RelayKeys     []TrustedRelayKey `json:"relay_keys" yaml:"relay_keys"`
	IssuedAt      string            `json:"issued_at" yaml:"issued_at"`
	ExpiresAt     string            `json:"expires_at" yaml:"expires_at"`
	Source        BundleSource      `json:"source" yaml:"source"`
	ImportedAt    string            `json:"imported_at" yaml:"imported_at"`
}

// TrustedRelayKey は信頼済みの署名鍵
type TrustedRelayKey struct {
	KeyID      string `json:"key_id" yaml:"key_id"`
	Thumbprint string `json:"thumbprint" yaml:"thumbprint"`
}

// BundleSource はバンドルの取得元情報
type BundleSource struct {
	FileName string `json:"file_name" yaml:"file_name"`
	SHA256   string `json:"sha256" yaml:"sha256"`
}

// FindTrustedBundle returns a trusted bundle by allowed domain.
func FindTrustedBundle(store *Store, allowedDomain string) *TrustedBundle {
	if store == nil {
		return nil
	}
	resolved := store.Resolved()
	for _, bundle := range resolved.Client.Trust.Bundles {
		if bundle.AllowedDomain == allowedDomain {
			b := bundle
			return &b
		}
	}
	return nil
}

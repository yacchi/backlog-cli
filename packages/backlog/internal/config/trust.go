package config

// ResolvedClient はマージ済みのクライアント設定
type ResolvedClient struct {
	Trust ResolvedClientTrust `json:"trust" jubako:"/client/trust"`
}

// ResolvedClientTrust は信頼設定
type ResolvedClientTrust struct {
	Bundles []TrustedBundle `json:"bundles" jubako:"/client/trust/bundles"`
}

// TrustedBundle はインポート済みのバンドル設定。
// バンドルは「信頼する中継サーバー（relay_url）と署名鍵」を束縛し、
// スペース（Backlog のスペース/ドメイン）は束縛しない。Name で一意に識別する。
type TrustedBundle struct {
	Name          string            `json:"name" yaml:"name"`
	RelayURL      string            `json:"relay_url" yaml:"relay_url"`
	BundleToken   string            `json:"bundle_token,omitempty" yaml:"bundle_token,omitempty"`
	RelayKeys     []TrustedRelayKey `json:"relay_keys" yaml:"relay_keys"`
	IssuedAt      string            `json:"issued_at" yaml:"issued_at"`
	ExpiresAt     string            `json:"expires_at" yaml:"expires_at"`
	CertsCacheTTL int               `json:"certs_cache_ttl" yaml:"certs_cache_ttl"`
	Source        BundleSource      `json:"source" yaml:"source"`
	ImportedAt    string            `json:"imported_at" yaml:"imported_at"`

	// Deprecated: v1 互換のための読み込み専用フィールド。
	// 旧 config.yaml の id / allowed_domain を Name に移送するためだけに用いる。
	LegacyID            string `json:"id,omitempty" yaml:"id,omitempty"`
	LegacyAllowedDomain string `json:"allowed_domain,omitempty" yaml:"allowed_domain,omitempty"`
}

// ResolvedName はバンドルの識別子を返す。Name が空の場合は v1 互換で
// allowed_domain / id を順に参照する。
func (b TrustedBundle) ResolvedName() string {
	if b.Name != "" {
		return b.Name
	}
	if b.LegacyAllowedDomain != "" {
		return b.LegacyAllowedDomain
	}
	return b.LegacyID
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

// FindTrustedBundleByName returns a trusted bundle by its name.
func FindTrustedBundleByName(store *Store, name string) *TrustedBundle {
	if store == nil || name == "" {
		return nil
	}
	resolved := store.Resolved()
	for _, bundle := range resolved.Client.Trust.Bundles {
		if bundle.ResolvedName() == name {
			b := bundle
			return &b
		}
	}
	return nil
}

// FindTrustedBundleByRelayURL returns a trusted bundle whose relay_url matches.
// 中継サーバーURLからバンドルを引く（Web設定フロー等、name が未確定の場面で使う）。
func FindTrustedBundleByRelayURL(store *Store, relayURL string) *TrustedBundle {
	if store == nil || relayURL == "" {
		return nil
	}
	resolved := store.Resolved()
	for _, bundle := range resolved.Client.Trust.Bundles {
		if relayURLMatches(bundle.RelayURL, relayURL) {
			b := bundle
			return &b
		}
	}
	return nil
}

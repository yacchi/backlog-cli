package config

// ResolvedTenant は中継サーバー向けのテナント設定
type ResolvedTenant struct {
	JWKS           string `json:"jwks" jubako:"jwks,env:SERVER_TENANT_{key}_JWKS,sensitive"`
	ActiveKeys     string `json:"active_keys" jubako:"active_keys,env:SERVER_TENANT_{key}_ACTIVE_KEYS"`
	AllowedDomain  string `json:"allowed_domain" jubako:"allowed_domain,env:SERVER_TENANT_{key}_ALLOWED_DOMAIN"`
	InfoTTL        int    `json:"info_ttl" jubako:"info_ttl,env:SERVER_TENANT_{key}_INFO_TTL"`
	PassphraseHash string `json:"passphrase_hash" jubako:"passphrase_hash,env:SERVER_TENANT_{key}_PASSPHRASE_HASH,sensitive"`
}

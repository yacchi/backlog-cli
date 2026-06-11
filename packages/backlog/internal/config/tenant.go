package config

// ResolvedTenant は中継サーバー向けのテナント設定。
// テナントはバンドル配布単位であり、マップのキー（name）で識別する。
// スペース（Backlog のスペース/ドメイン）とは無関係。
type ResolvedTenant struct {
	JWKS           string `json:"jwks" jubako:"jwks,env:SERVER_TENANT_{key}_JWKS,sensitive"`
	ActiveKeys     string `json:"active_keys" jubako:"active_keys,env:SERVER_TENANT_{key}_ACTIVE_KEYS"`
	InfoTTL        int    `json:"info_ttl" jubako:"info_ttl,env:SERVER_TENANT_{key}_INFO_TTL"`
	PassphraseHash string `json:"passphrase_hash" jubako:"passphrase_hash,env:SERVER_TENANT_{key}_PASSPHRASE_HASH,sensitive"`
}

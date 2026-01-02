package relay

import (
	"fmt"
	"net/http"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
)

// CacheType はキャッシュの種別
type CacheType int

const (
	// CacheTypeNone はキャッシュ不可（認証系、bundle等）
	CacheTypeNone CacheType = iota
	// CacheTypeShort は短期キャッシュ（info等）
	CacheTypeShort
	// CacheTypeLong は長期キャッシュ（certs等）
	CacheTypeLong
	// CacheTypeStatic は静的アセット（immutable）
	CacheTypeStatic
)

// デフォルトのキャッシュTTL（秒）
const (
	defaultShortTTL  = 300      // 5分
	defaultLongTTL   = 3600     // 1時間
	defaultStaticTTL = 31536000 // 1年
)

// SetCacheHeaders はレスポンスにキャッシュ制御ヘッダーを設定する
func SetCacheHeaders(w http.ResponseWriter, cacheType CacheType, cfg *config.Store) {
	switch cacheType {
	case CacheTypeNone:
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	case CacheTypeShort:
		ttl := config.GetDefault[int](cfg, config.PathServerCacheShortTtl, defaultShortTTL)
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, stale-while-revalidate=%d", ttl, ttl/2))
	case CacheTypeLong:
		ttl := config.GetDefault[int](cfg, config.PathServerCacheLongTtl, defaultLongTTL)
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, stale-while-revalidate=%d", ttl, ttl/2))
	case CacheTypeStatic:
		ttl := config.GetDefault[int](cfg, config.PathServerCacheStaticTtl, defaultStaticTTL)
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, immutable", ttl))
	}
}

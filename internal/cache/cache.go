package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cache はキャッシュインターフェース
type Cache interface {
	Get(key string, v interface{}) (bool, error)
	Set(key string, v interface{}, ttl time.Duration) error
	Clear() error
}

// FileCache はファイルベースのキャッシュ実装
type FileCache struct {
	dir string
}

type cacheItem struct {
	ExpiresAt time.Time   `json:"expires_at"`
	Data      interface{} `json:"data"`
}

// NewFileCache は新しいFileCacheを作成する
func NewFileCache(dir string) (*FileCache, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}

	return &FileCache{dir: dir}, nil
}

func (c *FileCache) getFilePath(key string) string {
	// キーの形式: "type:domain:extra..."
	// 例: "issue:backlog.jp:PROJ-1" -> "issue_backlog.jp_PROJ-1.json"
	
	// ファイル名に使用できない文字を置換
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			return r
		}
		return '_'
	}, key)

	// 長すぎる場合（一覧取得のオプションなど）は末尾をハッシュ化
	if len(safe) > 64 {
		hash := sha256.Sum256([]byte(key))
		safe = safe[:32] + "_" + hex.EncodeToString(hash[:])[:16]
	}

	return filepath.Join(c.dir, safe+".json")
}

// Get はキャッシュを取得する
// キャッシュが存在し、有効期限内であれば true と nil を返す
// キャッシュが存在しない、または期限切れの場合は false と nil を返す
func (c *FileCache) Get(key string, v interface{}) (bool, error) {
	path := c.getFilePath(key)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()

	// 期限切れチェックのため、まずはエンベロープだけデコードしたいが、
	// Dataの型がわからないとデコードできないため、一旦 json.RawMessage で受ける手もある。
	// ここではシンプルに、cacheItemのDataフィールドに v のポインタをセットしてデコードさせるのではなく、
	// 一旦 map[string]interface{} や RawMessage で受ける構造にする必要がある。

	// しかし、Goのjson.Unmarshalは interface{} にデコードすると map になるため、
	// 構造体に戻すのが面倒になる。

	// 正しいやり方:
	// cacheItem struct の Data フィールドを json.RawMessage にする。

	var item struct {
		ExpiresAt time.Time       `json:"expires_at"`
		Data      json.RawMessage `json:"data"`
	}

	if err := json.NewDecoder(f).Decode(&item); err != nil {
		// デコードエラーならキャッシュ無効扱い
		return false, nil
	}

	if time.Now().After(item.ExpiresAt) {
		// 期限切れならファイルを削除して false を返す
		_ = os.Remove(path)
		return false, nil
	}

	if err := json.Unmarshal(item.Data, v); err != nil {
		return false, fmt.Errorf("failed to unmarshal cache data: %w", err)
	}

	return true, nil
}

// Set はキャッシュを保存する
func (c *FileCache) Set(key string, v interface{}, ttl time.Duration) error {
	path := c.getFilePath(key)

	item := cacheItem{
		ExpiresAt: time.Now().Add(ttl),
		Data:      v,
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(item)
}

// Clear はキャッシュディレクトリを削除する
func (c *FileCache) Clear() error {
	return os.RemoveAll(c.dir)
}

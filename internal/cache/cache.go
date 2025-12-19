package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
// dirが空の場合はユーザーのキャッシュディレクトリを使用する
func NewFileCache(dir string) (*FileCache, error) {
	if dir == "" {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user cache dir: %w", err)
		}
		dir = filepath.Join(userCacheDir, "backlog-cli")
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}

	return &FileCache{dir: dir}, nil
}

func (c *FileCache) getFilePath(key string) string {
	hash := sha256.Sum256([]byte(key))
	filename := hex.EncodeToString(hash[:]) + ".json"
	return filepath.Join(c.dir, filename)
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

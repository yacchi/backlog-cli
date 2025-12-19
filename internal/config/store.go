package config

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/format/yaml"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/layer/env"
	"github.com/yacchi/jubako/layer/mapdata"
	"github.com/yacchi/jubako/source/bytes"
	"github.com/yacchi/jubako/source/fs"
)

// Store は設定のレイヤー管理を行うjubakoベースの実装
type Store struct {
	mu sync.RWMutex

	// メインの設定ストア
	store *jubako.Store[ResolvedConfig]

	// アクティブプロファイル名
	activeProfile string

	// プロジェクト設定ファイルのパス
	projectConfigPath string

	// クレデンシャルファイルのパス
	credentialsPath string
}

// SensitiveMaskString はセンシティブフィールドのマスク文字列
const SensitiveMaskString = "********"

// newConfigStore は新しいConfigStoreを作成する
// すべてのレイヤーを静的に追加する。ファイルが存在しない場合は空として扱う。
func newConfigStore() (*Store, error) {
	store := jubako.New[ResolvedConfig](
		// センシティブフィールドをマスクする
		jubako.WithSensitiveMaskString(SensitiveMaskString),
	)

	// Layer 1: Defaults (embedded YAML)
	if err := store.Add(
		layer.New(
			LayerDefaults,
			bytes.FromString(string(defaultConfigYAML)),
			yaml.New(),
		),
		jubako.WithReadOnly(),
		jubako.WithNoWatch(),
	); err != nil {
		return nil, err
	}

	// Layer 2: User config (~/.config/backlog/config.yaml)
	userConfigPath, err := configPath()
	if err != nil {
		return nil, err
	}
	if err := store.Add(
		layer.New(
			LayerUser,
			fs.New(userConfigPath),
			yaml.New(),
		),
		jubako.WithOptional(),
	); err != nil {
		return nil, err
	}

	// Layer 3: Credentials (~/.config/backlog/credentials.yaml)
	// - 最上位に配置することで、設定が誤って書き込まれることを防ぐ
	// - WithSensitive() により、センシティブフィールドのみ書き込み可能
	// - WithOptional() により、ファイルが存在しなくても空として扱う
	credentialsPath, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	if err := store.Add(
		layer.New(
			LayerCredentials,
			fs.New(credentialsPath, fs.WithFileMode(0600)),
			yaml.New(),
		),
		jubako.WithSensitive(),
		jubako.WithOptional(),
	); err != nil {
		return nil, err
	}

	// Layer 4: Project config (.backlog.yaml)
	// 見つかったらそのパス、なければカレントディレクトリの .backlog.yaml をデフォルト
	projectConfigPath, _ := findProjectConfigPath()
	if projectConfigPath == "" {
		projectConfigPath = ".backlog.yaml"
	}
	if err := store.Add(
		layer.New(
			LayerProject,
			fs.New(projectConfigPath),
			yaml.New(),
		),
		jubako.WithOptional(),
	); err != nil {
		return nil, err
	}

	// Layer 5: Environment variables
	// Uses schema-based mapping with jubako struct tags
	// WithEnvironFunc でショートカット環境変数を展開してから供給
	if err := store.Add(
		env.NewWithAutoSchema(LayerEnv, "BACKLOG_",
			env.WithEnvironFunc(expandEnvShortcuts),
		),
		jubako.WithReadOnly(),
	); err != nil {
		return nil, err
	}

	// Layer 6: Command-line flags
	// 静的に空のレイヤーを追加。SetFlagsLayer で値を設定
	if err := store.Add(
		mapdata.New(LayerArgs, nil),
	); err != nil {
		return nil, err
	}

	return &Store{
		store:             store,
		activeProfile:     DefaultProfile,
		projectConfigPath: projectConfigPath,
		credentialsPath:   credentialsPath,
	}, nil
}

// LoadAll は全レイヤーを読み込んでConfigを構築する
func (s *Store) LoadAll(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// メインストアをロード
	// 環境変数 BACKLOG_PROFILE は Jubako の env レイヤー経由で
	// resolved.Project.Profile にマッピングされる
	if err := s.store.Load(ctx); err != nil {
		return err
	}

	// プロジェクト設定/環境変数からアクティブプロファイルを設定
	resolved := s.store.Get()
	if resolved.Project.Profile != "" {
		s.activeProfile = resolved.Project.Profile
	}

	return nil
}

// SetFlagsLayer はコマンドラインフラグからのオーバーライドを設定する
// LayerArgs は静的に追加済みなので、SetTo で値を設定する
func (s *Store) SetFlagsLayer(options []jubako.SetOption) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.Set(LayerArgs, options...)
}

// Reload は設定を再読み込みする
func (s *Store) Reload(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.store.Reload(ctx)
}

// ====================
// アクセサ（読み取り）
// ====================

// Resolved は解決済み設定を返す
func (s *Store) Resolved() *ResolvedConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resolved := s.store.Get()
	resolved.ActiveProfile = s.activeProfile
	return &resolved
}

// GetActiveProfile はアクティブプロファイル名を返す
func (s *Store) GetActiveProfile() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeProfile
}

// SetActiveProfile はアクティブプロファイルを設定する
func (s *Store) SetActiveProfile(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeProfile = name
}

// Profile は指定プロファイルを取得する
func (s *Store) Profile(name string) *ResolvedProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resolved := s.store.Get()
	if name == "" {
		name = DefaultProfile
	}
	if p, ok := resolved.Profiles[name]; ok {
		return p
	}
	return resolved.Profiles[DefaultProfile]
}

// CurrentProfile はアクティブプロファイルを取得する
func (s *Store) CurrentProfile() *ResolvedProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resolved := s.store.Get()
	if s.activeProfile == "" {
		return resolved.Profiles[DefaultProfile]
	}
	if p, ok := resolved.Profiles[s.activeProfile]; ok {
		return p
	}
	return resolved.Profiles[DefaultProfile]
}

// Profiles は全プロファイルを取得する
func (s *Store) Profiles() map[string]*ResolvedProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.store.Get().Profiles
}

// Credential は指定プロファイルのクレデンシャルを取得する
func (s *Store) Credential(profileName string) *Credential {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if profileName == "" {
		profileName = DefaultProfile
	}
	resolved := s.store.Get()
	if resolved.Credentials == nil {
		return nil
	}
	return resolved.Credentials[profileName]
}

// CurrentCredential はアクティブプロファイルのクレデンシャルを取得する
func (s *Store) CurrentCredential() *Credential {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resolved := s.store.Get()
	if resolved.Credentials == nil {
		return nil
	}
	return resolved.Credentials[s.activeProfile]
}

// Server はサーバー設定を取得する
func (s *Store) Server() *ResolvedServer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resolved := s.store.Get()
	return &resolved.Server
}

// Display は表示設定を取得する
func (s *Store) Display() *ResolvedDisplay {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resolved := s.store.Get()
	return &resolved.Display
}

// Auth は認証設定を取得する
func (s *Store) Auth() *ResolvedAuth {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resolved := s.store.Get()
	return &resolved.Auth
}

// Project はプロジェクト設定を取得する
func (s *Store) Project() *ResolvedProject {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resolved := s.store.Get()
	return &resolved.Project
}

// GetProjectConfigPath はプロジェクト設定ファイルのパスを返す
func (s *Store) GetProjectConfigPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.projectConfigPath
}

// GetUserConfigPath はユーザー設定ファイルのパスを返す
func (s *Store) GetUserConfigPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if info := s.store.GetLayerInfo(LayerUser); info != nil {
		return info.Path()
	}
	return ""
}

// GetCredentialsPath はクレデンシャルファイルのパスを返す
func (s *Store) GetCredentialsPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.credentialsPath
}

// GetProjectRoot はプロジェクトルートディレクトリを返す
// プロジェクト設定ファイルのパスからディレクトリを取得し、
// 見つからない場合は .git を検索する
func (s *Store) GetProjectRoot() (string, error) {
	s.mu.RLock()
	projectPath := s.projectConfigPath
	s.mu.RUnlock()

	// プロジェクト設定パスがあれば、そのディレクトリを返す
	if projectPath != "" && filepath.IsAbs(projectPath) {
		return filepath.Dir(projectPath), nil
	}

	// .git を検索
	return findGitRoot()
}

// SetProjectConfigPath はプロジェクト設定ファイルのパスを設定する
func (s *Store) SetProjectConfigPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projectConfigPath = path
}

// BacklogApp は指定ドメインのBacklogアプリ設定を取得する
func (s *Store) BacklogApp(domain string) *ResolvedBacklogApp {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resolved := s.store.Get()
	for key := range resolved.Server.Backlog {
		app := resolved.Server.Backlog[key]
		if app.DomainValue == domain {
			return &app
		}
	}
	return nil
}

// ====================
// セッター（書き込み）
// ====================

// SetCredential はクレデンシャルを設定する
// クレデンシャルは専用のクレデンシャルレイヤーにのみ書き込まれる
func (s *Store) SetCredential(profileName string, cred *Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.store.Set(LayerCredentials,
		jubako.Struct(PathCredential+"/"+profileName, cred),
		jubako.SkipZeroValues(),
	)
}

// DeleteCredential はクレデンシャルを削除する
func (s *Store) DeleteCredential(profileName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store.GetLayer(LayerCredentials) == nil {
		return nil // クレデンシャルレイヤーがなければ何もしない
	}

	// コンテナパスで一括削除
	return s.store.DeleteFrom(LayerCredentials, PathCredential+"/"+profileName)
}

// SetProfileValue はプロファイルの値を設定する
func (s *Store) SetProfileValue(layerName, profileName, field string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := "/profile/" + profileName + "/" + field
	return s.store.SetTo(layer.Name(layerName), path, value)
}

// SetProjectValue はプロジェクト設定の値を設定する
func (s *Store) SetProjectValue(layerName, field string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := "/project/" + field
	return s.store.SetTo(layer.Name(layerName), path, value)
}

// Set はドット区切りのキーで値を設定する（ユーザーレイヤー）
func (s *Store) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.store.SetTo(LayerUser, DotToPointer(key), value)
}

// ====================
// 保存
// ====================

// Save は更新があったレイヤーを保存する
func (s *Store) Save(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.store.Save(ctx)
}

// SaveCredentials はクレデンシャルのみを保存する
func (s *Store) SaveCredentials(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// クレデンシャルレイヤーがなければ何もしない
	if s.store.GetLayer(LayerCredentials) == nil {
		return nil
	}

	// メインストアのSaveを使用（dirtyなレイヤーのみ保存される）
	return s.store.Save(ctx)
}

// ====================
// CLIコマンド用メソッド
// ====================

// Get は指定キーの値を取得する（CLIコマンド用）
func (s *Store) Get(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rv := s.store.GetAt(DotToPointer(key))
	if rv.Exists {
		return rv.Value
	}
	return nil
}

// WalkFunc は Walk で使用するコールバック関数の型
// path: ドット区切りのパス（例: profile.default.space）
// value: マスク済みの値
// layerName: 値の出所となるレイヤー名
type WalkFunc func(path string, value any, layerName string) bool

// Walk は全設定パスをイテレートする
// パスはドット区切り形式（例: profile.default.space）
// センシティブフィールドはマスクされた値が渡される
// fn が false を返すとイテレーションを停止する
func (s *Store) Walk(fn WalkFunc) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.store.Walk(func(ctx jubako.WalkContext) bool {
		rv := ctx.Value() // マスク済み
		if !rv.Exists {
			return true
		}
		// /profile/default/space → profile.default.space
		key := strings.ReplaceAll(ctx.Path[1:], "/", ".")
		layerName := ""
		if rv.Layer != nil {
			layerName = string(rv.Layer.Name())
		}
		return fn(key, rv.Value, layerName)
	})
}

// WalkEntry は Walk で返されるエントリ情報
type WalkEntry struct {
	Path         string // ドット区切りのパス
	Value        any    // マスク済みの値
	Layer        string // 値の出所となるレイヤー名
	DefaultValue any    // デフォルト値（存在しない場合は nil）
}

// WalkExFunc は WalkEx で使用するコールバック関数の型
type WalkExFunc func(entry WalkEntry) bool

// WalkEx は全設定パスをイテレートし、デフォルト値も含めたエントリ情報を返す
func (s *Store) WalkEx(fn WalkExFunc) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.store.Walk(func(ctx jubako.WalkContext) bool {
		rv := ctx.Value() // マスク済み
		if !rv.Exists {
			return true
		}
		// /profile/default/space → profile.default.space
		key := strings.ReplaceAll(ctx.Path[1:], "/", ".")
		layerName := ""
		if rv.Layer != nil {
			layerName = string(rv.Layer.Name())
		}

		// デフォルト値を取得
		var defaultValue any
		for _, v := range ctx.AllValues() {
			if v.Layer != nil && string(v.Layer.Name()) == LayerDefaults {
				defaultValue = v.Value
				break
			}
		}

		return fn(WalkEntry{
			Path:         key,
			Value:        rv.Value,
			Layer:        layerName,
			DefaultValue: defaultValue,
		})
	})
}

// SetToLayer は指定レイヤーに値を設定する（CLIコマンド用）
func (s *Store) SetToLayer(layerName, key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.store.SetTo(layer.Name(layerName), DotToPointer(key), value)
}

// ====================
// グローバルストア管理
// ====================

var (
	globalStore   *Store
	globalStoreMu sync.RWMutex
)

// Load はグローバル設定ストアを初期化してロードする
// すでにロード済みの場合は既存のストアを返す
func Load(ctx context.Context) (*Store, error) {
	globalStoreMu.Lock()
	defer globalStoreMu.Unlock()

	if globalStore != nil {
		return globalStore, nil
	}

	store, err := newConfigStore()
	if err != nil {
		return nil, err
	}

	if err := store.LoadAll(ctx); err != nil {
		return nil, err
	}

	globalStore = store
	return globalStore, nil
}

// ResetConfig はグローバル設定ストアをリセットする（テスト用）
func ResetConfig() {
	globalStoreMu.Lock()
	defer globalStoreMu.Unlock()
	globalStore = nil
}

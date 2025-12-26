package config

import (
	"testing"

	"github.com/yacchi/jubako"
)

func TestJubakoStoreLoad(t *testing.T) {
	// テスト用に環境変数をクリア（t.Setenvは元の値を自動復元する）
	t.Setenv("BACKLOG_SPACE", "")

	ctx := t.Context()

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("NewJubakoConfigStore failed: %v", err)
	}

	if err := store.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	resolved := store.Resolved()
	if resolved == nil {
		t.Fatal("Resolved returned nil")
	}

	// デフォルトプロファイルが存在することを確認
	if resolved.ActiveProfile != DefaultProfile {
		t.Errorf("ActiveProfile = %q, want %q", resolved.ActiveProfile, DefaultProfile)
	}

	// デフォルト設定が適用されていることを確認
	server := store.Server()
	if server == nil {
		t.Fatal("Server returned nil")
	}
	if server.Port == 0 {
		t.Error("Server.Port should have a default value")
	}

	display := store.Display()
	if display == nil {
		t.Fatal("Display returned nil")
	}
}

func TestJubakoStoreEnvOverrides(t *testing.T) {
	// 環境変数を設定（動的マッピング形式: BACKLOG_PROFILE_{key}_SPACE）
	t.Setenv("BACKLOG_PROFILE_default_SPACE", "test-space-from-env")
	t.Setenv("BACKLOG_CLIENT_ID_JP", "test-client-id-jp")

	ctx := t.Context()

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("NewJubakoConfigStore failed: %v", err)
	}

	if err := store.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	// プロファイル設定の確認
	profile := store.CurrentProfile()
	if profile == nil {
		t.Fatal("CurrentProfile returned nil")
	}

	if profile.Space != "test-space-from-env" {
		t.Errorf("Space = %q, want %q", profile.Space, "test-space-from-env")
	}

	// Backlogアプリ設定の確認
	// 注: BACKLOG_CLIENT_ID_JP は動的マッピングが必要だが、ResolvedBacklogApp には
	// 環境変数タグが設定されていないため、このテストはスキップ
	// TODO: ResolvedBacklogApp に環境変数マッピングを追加する
}

func TestJubakoStoreReload(t *testing.T) {
	ctx := t.Context()

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("NewJubakoConfigStore failed: %v", err)
	}

	if err := store.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	// Reloadしても問題ないことを確認
	if err := store.Reload(ctx); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
}

func TestJubakoStoreActiveProfile(t *testing.T) {
	ctx := t.Context()

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("NewJubakoConfigStore failed: %v", err)
	}

	if err := store.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	// デフォルトプロファイルを確認
	if got := store.GetActiveProfile(); got != DefaultProfile {
		t.Errorf("GetActiveProfile = %q, want %q", got, DefaultProfile)
	}

	// プロファイルを変更
	store.SetActiveProfile("custom")
	if got := store.GetActiveProfile(); got != "custom" {
		t.Errorf("GetActiveProfile after SetActiveProfile = %q, want %q", got, "custom")
	}
}

func TestEnvShortcuts(t *testing.T) {
	// ショートカット環境変数のテスト
	// BACKLOG_SPACE は BACKLOG_PROFILE_default_SPACE に展開される
	t.Setenv("BACKLOG_SPACE", "shortcut-test-space")

	ctx := t.Context()

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("NewConfigStore failed: %v", err)
	}

	if err := store.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	profile := store.CurrentProfile()
	if profile == nil {
		t.Fatal("CurrentProfile returned nil")
	}

	if profile.Space != "shortcut-test-space" {
		t.Errorf("Space = %q, want %q", profile.Space, "shortcut-test-space")
	}
}

func TestEnvShortcutsPriority(t *testing.T) {
	// 完全形式が設定されている場合は、ショートカットより優先
	t.Setenv("BACKLOG_SPACE", "shortcut-space")
	t.Setenv("BACKLOG_PROFILE_default_SPACE", "full-form-space")

	ctx := t.Context()

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("NewConfigStore failed: %v", err)
	}

	if err := store.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	profile := store.CurrentProfile()
	if profile == nil {
		t.Fatal("CurrentProfile returned nil")
	}

	// 完全形式が優先される
	if profile.Space != "full-form-space" {
		t.Errorf("Space = %q, want %q (full form should take priority)", profile.Space, "full-form-space")
	}
}

func TestSetFlagsLayer(t *testing.T) {
	ctx := t.Context()

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("NewJubakoConfigStore failed: %v", err)
	}

	if err := store.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	// 初期値を確認
	profile := store.CurrentProfile()
	if profile == nil {
		t.Fatal("CurrentProfile returned nil")
	}
	t.Logf("Before: Project = %q", profile.Project)

	// SetFlagsLayerを呼ぶ（jubako.SetOptionを使用）
	setOptions := []jubako.SetOption{
		jubako.String(PathProjectName, "QS2"),
	}
	if err := store.SetFlagsLayer(setOptions); err != nil {
		t.Fatalf("SetFlagsLayer failed: %v", err)
	}

	// 値が反映されているか確認
	project := store.Project()
	if project == nil {
		t.Fatal("Project returned nil after SetFlagsLayer")
	}
	t.Logf("After: Project = %q", project.Name)

	if project.Name != "QS2" {
		t.Errorf("Project = %q, want %q", project.Name, "QS2")
	}
}

func TestGet(t *testing.T) {
	ctx := t.Context()

	// 環境変数をクリア
	t.Setenv("BACKLOG_SPACE", "")

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("NewConfigStore failed: %v", err)
	}

	if err := store.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	t.Run("non-existing value", func(t *testing.T) {
		val, ok := Get[int](store, "/non/existing/path")
		if ok {
			t.Error("Get should return ok=false for non-existing path")
		}
		if val != 0 {
			t.Errorf("Get should return zero value for non-existing path, got %d", val)
		}
	})

	t.Run("type mismatch", func(t *testing.T) {
		// ServerHost は string なので int として取得すると失敗する
		val, ok := Get[int](store, PathServerHost)
		if ok {
			t.Error("Get should return ok=false for type mismatch")
		}
		if val != 0 {
			t.Errorf("Get should return zero value for type mismatch, got %d", val)
		}
	})

	t.Run("nil store", func(t *testing.T) {
		val, ok := Get[string](nil, PathServerHost)
		if ok {
			t.Error("Get should return ok=false for nil store")
		}
		if val != "" {
			t.Errorf("Get should return zero value for nil store, got %q", val)
		}
	})
}

func TestGetAllConstantPaths(t *testing.T) {
	ctx := t.Context()

	// 環境変数をクリア
	t.Setenv("BACKLOG_SPACE", "")

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("NewConfigStore failed: %v", err)
	}

	if err := store.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	// スカラー値のパス（defaults.yamlにデフォルト値が設定されているもの）
	// 型ごとにテストケースを分類
	stringPaths := []struct {
		name string
		path string
	}{
		{"PathServerHost", PathServerHost},
		{"PathServerBaseUrl", PathServerBaseUrl},
		{"PathServerAllowedHostPatterns", PathServerAllowedHostPatterns},
		{"PathDisplayTimezone", PathDisplayTimezone},
		{"PathDisplayDateFormat", PathDisplayDateFormat},
		{"PathDisplayDatetimeFormat", PathDisplayDatetimeFormat},
		{"PathServerAuditOutput", PathServerAuditOutput},
		{"PathServerAuditFilePath", PathServerAuditFilePath},
		{"PathServerAuditWebhookUrl", PathServerAuditWebhookUrl},
		{"PathCacheDir", PathCacheDir},
		{"PathProjectProfile", PathProjectProfile},
		{"PathProjectSpace", PathProjectSpace},
		{"PathProjectDomain", PathProjectDomain},
		{"PathProjectName", PathProjectName},
	}

	intPaths := []struct {
		name string
		path string
	}{
		{"PathServerPort", PathServerPort},
		{"PathServerHttpReadTimeout", PathServerHttpReadTimeout},
		{"PathServerHttpWriteTimeout", PathServerHttpWriteTimeout},
		{"PathServerHttpIdleTimeout", PathServerHttpIdleTimeout},
		{"PathServerJwtExpiry", PathServerJwtExpiry},
		{"PathServerCacheShortTtl", PathServerCacheShortTtl},
		{"PathServerCacheLongTtl", PathServerCacheLongTtl},
		{"PathServerCacheStaticTtl", PathServerCacheStaticTtl},
		{"PathServerRateLimitRequestsPerMinute", PathServerRateLimitRequestsPerMinute},
		{"PathServerRateLimitBurst", PathServerRateLimitBurst},
		{"PathServerRateLimitCleanupInterval", PathServerRateLimitCleanupInterval},
		{"PathServerRateLimitEntryTtl", PathServerRateLimitEntryTtl},
		{"PathServerAuditWebhookTimeout", PathServerAuditWebhookTimeout},
		{"PathDisplaySummaryMaxLength", PathDisplaySummaryMaxLength},
		{"PathDisplaySummaryCommentCount", PathDisplaySummaryCommentCount},
		{"PathDisplayDefaultCommentCount", PathDisplayDefaultCommentCount},
		{"PathDisplayDefaultIssueLimit", PathDisplayDefaultIssueLimit},
		{"PathDisplayMarkdownCacheExcerpt", PathDisplayMarkdownCacheExcerpt},
		{"PathAuthMinCallbackPort", PathAuthMinCallbackPort},
		{"PathAuthMaxCallbackPort", PathAuthMaxCallbackPort},
		{"PathAuthSessionCheckInterval", PathAuthSessionCheckInterval},
		{"PathAuthSessionTimeout", PathAuthSessionTimeout},
		{"PathAuthKeepaliveInterval", PathAuthKeepaliveInterval},
		{"PathAuthKeepaliveTimeout", PathAuthKeepaliveTimeout},
		{"PathAuthKeepaliveConnectTimeout", PathAuthKeepaliveConnectTimeout},
		{"PathAuthKeepaliveGracePeriod", PathAuthKeepaliveGracePeriod},
		{"PathCacheTtl", PathCacheTtl},
	}

	boolPaths := []struct {
		name string
		path string
	}{
		{"PathServerRateLimitEnabled", PathServerRateLimitEnabled},
		{"PathServerAuditEnabled", PathServerAuditEnabled},
		{"PathDisplayHyperlink", PathDisplayHyperlink},
		{"PathDisplayMarkdownView", PathDisplayMarkdownView},
		{"PathDisplayMarkdownWarn", PathDisplayMarkdownWarn},
		{"PathDisplayMarkdownCache", PathDisplayMarkdownCache},
		{"PathDisplayMarkdownCacheRaw", PathDisplayMarkdownCacheRaw},
		{"PathCacheEnabled", PathCacheEnabled},
	}

	t.Run("string paths", func(t *testing.T) {
		for _, tc := range stringPaths {
			t.Run(tc.name, func(t *testing.T) {
				_, ok := Get[string](store, tc.path)
				if !ok {
					t.Errorf("Get[string](%s) should return ok=true", tc.path)
				}
			})
		}
	})

	t.Run("int paths", func(t *testing.T) {
		for _, tc := range intPaths {
			t.Run(tc.name, func(t *testing.T) {
				_, ok := Get[int](store, tc.path)
				if !ok {
					t.Errorf("Get[int](%s) should return ok=true", tc.path)
				}
			})
		}
	})

	t.Run("bool paths", func(t *testing.T) {
		for _, tc := range boolPaths {
			t.Run(tc.name, func(t *testing.T) {
				_, ok := Get[bool](store, tc.path)
				if !ok {
					t.Errorf("Get[bool](%s) should return ok=true", tc.path)
				}
			})
		}
	})
}

func TestGetDefault(t *testing.T) {
	ctx := t.Context()

	// 環境変数をクリア
	t.Setenv("BACKLOG_SPACE", "")

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("NewConfigStore failed: %v", err)
	}

	if err := store.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	t.Run("non-existing value returns default", func(t *testing.T) {
		val := GetDefault(store, "/non/existing/path", 42)
		if val != 42 {
			t.Errorf("GetDefault should return default value for non-existing path, got %d", val)
		}
	})

	t.Run("nil store returns default", func(t *testing.T) {
		val := GetDefault[int](nil, PathServerPort, 9999)
		if val != 9999 {
			t.Errorf("GetDefault with nil store should return default 9999, got %d", val)
		}
	})

	t.Run("string default", func(t *testing.T) {
		val := GetDefault(store, "/non/existing/string", "fallback")
		if val != "fallback" {
			t.Errorf("GetDefault should return 'fallback', got %q", val)
		}
	})
}

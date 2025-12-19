package config

import (
	"context"
	"testing"

	"github.com/yacchi/jubako"
)

func TestJubakoStoreLoad(t *testing.T) {
	// テスト用に環境変数をクリア（t.Setenvは元の値を自動復元する）
	t.Setenv("BACKLOG_SPACE", "")

	ctx := context.Background()

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

	ctx := context.Background()

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
	ctx := context.Background()

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
	ctx := context.Background()

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

	ctx := context.Background()

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

	ctx := context.Background()

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
	ctx := context.Background()

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

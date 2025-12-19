package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	// グローバルストアをリセット
	ResetConfig()
	defer ResetConfig()

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	profile := cfg.CurrentProfile()
	if profile.Domain != "backlog.jp" {
		t.Errorf("Domain = %v, want %v", profile.Domain, "backlog.jp")
	}

	if profile.Output != "table" {
		t.Errorf("Output = %v, want %v", profile.Output, "table")
	}
}

func TestEnvOverrides(t *testing.T) {
	// グローバルストアをリセット
	ResetConfig()
	defer ResetConfig()

	// 動的マッピング形式: BACKLOG_PROFILE_{key}_SPACE
	t.Setenv("BACKLOG_PROFILE_default_SPACE", "test-space")

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	profile := cfg.CurrentProfile()
	if profile.Space != "test-space" {
		t.Errorf("Space = %v, want %v", profile.Space, "test-space")
	}
}

func TestStoreProjectConfigPath(t *testing.T) {
	// テスト用ディレクトリ作成
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub", "dir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// .backlog.yaml作成（新形式）
	configPath := filepath.Join(tmpDir, ".backlog.yaml")
	if err := os.WriteFile(configPath, []byte("project:\n  name: TEST-PROJ\n"), 0644); err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	// サブディレクトリに移動
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }() // cleanup: エラーは無視して良い

	// グローバルストアをリセット
	ResetConfig()
	defer ResetConfig()

	// Store をロード
	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// プロジェクト設定パスを取得
	projectPath := cfg.GetProjectConfigPath()
	if projectPath == "" {
		t.Fatal("GetProjectConfigPath() returned empty")
	}

	// macOSでは /var が /private/var へのシンボリックリンクのため、
	// EvalSymlinks で実パスを解決して比較
	expectedPath, err := filepath.EvalSymlinks(configPath)
	if err != nil {
		t.Fatalf("failed to resolve expected path: %v", err)
	}
	actualPath, err := filepath.EvalSymlinks(projectPath)
	if err != nil {
		t.Fatalf("failed to resolve actual path: %v", err)
	}
	if actualPath != expectedPath {
		t.Errorf("Path = %v, want %v", actualPath, expectedPath)
	}

	// プロジェクト設定の値を確認
	project := cfg.Project()
	if project.Name != "TEST-PROJ" {
		t.Errorf("Name = %v, want %v", project.Name, "TEST-PROJ")
	}
}

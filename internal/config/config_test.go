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
	os.MkdirAll(subDir, 0755)

	// .backlog.yaml作成（新形式）
	configPath := filepath.Join(tmpDir, ".backlog.yaml")
	os.WriteFile(configPath, []byte("project:\n  name: TEST-PROJ\n"), 0644)

	// サブディレクトリに移動
	oldDir, _ := os.Getwd()
	os.Chdir(subDir)
	defer os.Chdir(oldDir)

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
	expectedPath, _ := filepath.EvalSymlinks(configPath)
	actualPath, _ := filepath.EvalSymlinks(projectPath)
	if actualPath != expectedPath {
		t.Errorf("Path = %v, want %v", actualPath, expectedPath)
	}

	// プロジェクト設定の値を確認
	project := cfg.Project()
	if project.Name != "TEST-PROJ" {
		t.Errorf("Name = %v, want %v", project.Name, "TEST-PROJ")
	}
}

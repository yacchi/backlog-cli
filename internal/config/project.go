package config

import (
	"os"
	"path/filepath"
)

// ProjectConfigFiles は検索するファイル名の優先順
var ProjectConfigFiles = []string{
	".backlog.yaml",
	".backlog.yml",
	".backlog-project.yaml",
	".backlog-project.yml",
}

// DefaultProjectConfigFile はデフォルトのプロジェクト設定ファイル名
const DefaultProjectConfigFile = ".backlog.yaml"

// findProjectConfigPath はカレントディレクトリから上に向かって
// .backlog.yaml を検索し、パスを返す
func findProjectConfigPath() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		for _, name := range ProjectConfigFiles {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// ルートに到達、見つからず
			return "", nil
		}
		dir = parent
	}
}

// findGitRoot はカレントディレクトリから上に向かって
// .git ディレクトリを検索し、見つかったディレクトリを返す
func findGitRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		gitPath := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			// .gitはディレクトリまたはファイル（worktreeの場合）
			if info.IsDir() || info.Mode().IsRegular() {
				return dir, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// ルートに到達、見つからず
			return "", nil
		}
		dir = parent
	}
}

// GetProjectConfigPathForRoot は指定されたルートディレクトリのプロジェクト設定ファイルパスを返す
func GetProjectConfigPathForRoot(root string) string {
	// 既存のファイルがあればそれを使用
	for _, name := range ProjectConfigFiles {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	// なければデフォルトのファイル名を使用
	return filepath.Join(root, DefaultProjectConfigFile)
}

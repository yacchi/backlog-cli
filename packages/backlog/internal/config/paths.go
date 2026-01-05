package config

import (
	"os"
	"path/filepath"
	"strings"
)

// AppName is the application name used for config directories
const AppName = "backlog"

// configDir returns the config directory path (~/.config/backlog)
// Uses XDG_CONFIG_HOME if set, otherwise falls back to ~/.config
func configDir() (string, error) {
	// XDG_CONFIG_HOME を優先
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, AppName), nil
	}

	// フォールバック: ~/.config (XDG仕様のデフォルト)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".config", AppName), nil
}

// configPath returns the user config file path (~/.config/backlog/config.yaml)
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// credentialsPath はクレデンシャルファイルのパスを返す
func credentialsPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.yaml"), nil
}

// defaultCacheDir returns the default cache directory path
func defaultCacheDir() (string, error) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userCacheDir, AppName), nil
}

// DotToPointer converts a dot-separated path to a JSON Pointer.
// Example: "profile.default.space" -> "/profile/default/space"
func DotToPointer(dotPath string) string {
	return "/" + strings.ReplaceAll(dotPath, ".", "/")
}

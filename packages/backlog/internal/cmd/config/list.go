package config

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
)

var listAllFlag bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configuration values",
	Long: `List configuration values.

By default, shows only modified values (non-default).
Use --all to show all configuration values including defaults.`,
	RunE: runList,
}

func init() {
	listCmd.Flags().BoolVarP(&listAllFlag, "all", "a", false, "Show all configuration values including defaults")
}

func runList(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 設定ファイルのパスを表示
	if userPath := cfg.GetUserConfigPath(); userPath != "" {
		fmt.Printf("# User config: %s\n", userPath)
	}
	if credPath := cfg.GetCredentialsPath(); credPath != "" {
		fmt.Printf("# Credentials: %s\n", credPath)
	}
	if projectPath := cfg.GetProjectConfigPath(); projectPath != "" {
		fmt.Printf("# Project config: %s\n", projectPath)
	}

	// Walk でフィルタリングしながら収集（ソート済み）
	type entry struct {
		path         string
		value        string
		layer        string
		defaultValue string
	}
	var entries []entry
	maxWidth := 0

	cfg.WalkEx(func(e config.WalkEntry) bool {
		// --all でない場合、defaults レイヤーの値はスキップ
		if !listAllFlag && e.Layer == config.LayerDefaults {
			return true
		}
		valueStr := fmt.Sprintf("%v", e.Value)
		line := fmt.Sprintf("%s=%s", e.Path, valueStr)
		if len(line) > maxWidth {
			maxWidth = len(line)
		}
		defaultStr := ""
		if e.DefaultValue != nil {
			defaultStr = fmt.Sprintf("%v", e.DefaultValue)
		}
		entries = append(entries, entry{
			path:         e.Path,
			value:        valueStr,
			layer:        e.Layer,
			defaultValue: defaultStr,
		})
		return true
	})

	// 縦位置を揃えて出力
	for _, e := range entries {
		line := fmt.Sprintf("%s=%s", e.path, e.value)
		comment := e.layer
		if e.defaultValue != "" && e.value != e.defaultValue {
			comment = fmt.Sprintf("%s, default: %s", e.layer, e.defaultValue)
		}
		fmt.Printf("%-*s  # %s\n", maxWidth, line, comment)
	}

	if len(entries) == 0 {
		if listAllFlag {
			fmt.Println("No configuration values found.")
		} else {
			fmt.Println("No modified configuration values.")
			fmt.Println("Use --all to show all configuration values including defaults.")
		}
	}

	return nil
}

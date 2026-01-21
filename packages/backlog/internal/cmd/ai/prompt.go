package ai

import (
	"github.com/spf13/cobra"
)

// promptCmd はプロンプト関連コマンドの親コマンド
var promptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Manage AI summary prompts",
	Long:  "View, optimize, and manage AI summary prompt templates.",
}

func init() {
	promptCmd.AddCommand(optimizeCmd)
	promptCmd.AddCommand(applyCmd)
}

package ai

import (
	"github.com/spf13/cobra"
)

// AICmd はAI関連コマンドの親コマンド
var AICmd = &cobra.Command{
	Use:   "ai",
	Short: "AI-related commands",
	Long:  "Manage AI-powered features like summary prompt optimization.",
}

func init() {
	AICmd.AddCommand(promptCmd)
}

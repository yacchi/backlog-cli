package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
	"golang.org/x/term"
)

const (
	// claudePluginName は Claude Code プラグインの名前
	claudePluginName = "backlog-cli"
	// claudeMarketplaceRepo はプラグインを配布する Marketplace の GitHub リポジトリ
	claudeMarketplaceRepo = "yacchi/claude-plugins"
	// claudeMarketplaceName は Marketplace 追加後に割り当てられる名前
	claudeMarketplaceName = "yacchi-plugins"
)

// claudePluginInfo は `claude plugin list --json` の出力（必要な項目のみ）
type claudePluginInfo struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// maybeInstallClaudePlugin はセットアップ完了後に Claude Code プラグインの
// インストール/更新を提案する。Claude Code (claude コマンド) が見つからない場合や
// 非対話セッションの場合は何もしない（エラーも返さない）。
func maybeInstallClaudePlugin(cmd *cobra.Command) {
	// 対話モードでなければ何もしない（curl|sh などで勝手に claude を起動しない）
	if cmdutil.SkipConfirmation(cmd) || !term.IsTerminal(int(syscall.Stdin)) {
		return
	}

	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		// Claude Code がインストールされていない
		return
	}

	plugin, err := findInstalledClaudePlugin(cmd, claudeBin)
	if err != nil {
		// プラグイン一覧の取得に失敗した場合は黙ってスキップ
		return
	}

	if plugin != nil {
		promptUpdateClaudePlugin(cmd, claudeBin, plugin)
		return
	}

	promptInstallClaudePlugin(cmd, claudeBin)
}

// findInstalledClaudePlugin は backlog-cli プラグインが導入済みなら、その情報を返す。
// 未導入の場合は nil を返す。
func findInstalledClaudePlugin(cmd *cobra.Command, claudeBin string) (*claudePluginInfo, error) {
	out, err := exec.CommandContext(cmd.Context(), claudeBin, "plugin", "list", "--json").Output()
	if err != nil {
		return nil, err
	}

	var plugins []claudePluginInfo
	if err := json.Unmarshal(out, &plugins); err != nil {
		return nil, err
	}

	for _, p := range plugins {
		// id は "backlog-cli@yacchi-plugins" の形式
		name := p.ID
		if i := strings.IndexByte(name, '@'); i >= 0 {
			name = name[:i]
		}
		if name == claudePluginName {
			return &p, nil
		}
	}
	return nil, nil
}

// promptInstallClaudePlugin は未導入時にインストールを提案する
func promptInstallClaudePlugin(cmd *cobra.Command, claudeBin string) {
	ui.Info("Claude Code が見つかりました。")

	approved, err := ui.Confirm("Backlog の Claude Code プラグインをインストールしますか?", true)
	if err != nil || !approved {
		return
	}

	// Marketplace を追加（既に追加済みでも冪等）
	ui.Info("Marketplace を追加しています...")
	if err := runClaude(cmd, claudeBin, "plugin", "marketplace", "add", claudeMarketplaceRepo); err != nil {
		ui.Warning("Marketplace の追加に失敗しました: %v", err)
		return
	}

	ui.Info("プラグインをインストールしています...")
	target := claudePluginName + "@" + claudeMarketplaceName
	if err := runClaude(cmd, claudeBin, "plugin", "install", target); err != nil {
		ui.Warning("プラグインのインストールに失敗しました: %v", err)
		return
	}

	ui.Success("Claude Code プラグインをインストールしました（反映には Claude Code の再起動が必要な場合があります）")
}

// promptUpdateClaudePlugin は導入済み時にプラグインを更新する
func promptUpdateClaudePlugin(cmd *cobra.Command, claudeBin string, plugin *claudePluginInfo) {
	ui.Info("Claude Code プラグインを更新しています (v%s)...", plugin.Version)
	if err := runClaude(cmd, claudeBin, "plugin", "update", plugin.ID); err != nil {
		ui.Warning("プラグインの更新に失敗しました: %v", err)
		return
	}

	ui.Success("Claude Code プラグインを更新しました（反映には Claude Code の再起動が必要な場合があります）")
}

// runClaude は claude サブコマンドを実行し、出力をそのままターミナルへ流す
func runClaude(cmd *cobra.Command, claudeBin string, args ...string) error {
	c := exec.CommandContext(cmd.Context(), claudeBin, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}

package issue_type

import (
	"context"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
)

// IssueTypeCmd は課題種別を管理するコマンド
var IssueTypeCmd = &cobra.Command{
	Use:     "issue-type",
	Aliases: []string{"type"},
	Short:   "Manage issue types",
	Long:    "Work with Backlog issue types (種別).",
}

func init() {
	IssueTypeCmd.AddCommand(listCmd)
	IssueTypeCmd.AddCommand(viewCmd)
	IssueTypeCmd.AddCommand(createCmd)
	IssueTypeCmd.AddCommand(editCmd)
	IssueTypeCmd.AddCommand(deleteCmd)
}

// IssueTypeColor は種別の色情報
type IssueTypeColor struct {
	Hex  string
	Name string
}

// IssueTypeColors はBacklog APIで許可されている背景色
var IssueTypeColors = []IssueTypeColor{
	{"#e30000", "赤"},
	{"#990000", "暗い赤"},
	{"#934981", "紫"},
	{"#814fbc", "青紫"},
	{"#2779ca", "青"},
	{"#007e9a", "青緑"},
	{"#7ea800", "黄緑"},
	{"#ff9200", "オレンジ"},
	{"#ff3265", "ピンク"},
	{"#666665", "グレー"},
}

// GetColorName は色のHEX値から日本語名を返す
func GetColorName(hex string) string {
	for _, c := range IssueTypeColors {
		if c.Hex == hex {
			return c.Name
		}
	}
	return hex
}

// resolveIssueType はIDまたは名前から種別を解決する
func resolveIssueType(ctx context.Context, client *api.Client, projectKey, idOrName string) (*api.IssueType, error) {
	issueTypes, err := client.GetIssueTypes(ctx, projectKey)
	if err != nil {
		return nil, err
	}

	// IDとして解釈を試みる
	if id, err := strconv.Atoi(idOrName); err == nil {
		for i := range issueTypes {
			if issueTypes[i].ID == id {
				return &issueTypes[i], nil
			}
		}
	}

	// 名前として検索
	for i := range issueTypes {
		if issueTypes[i].Name == idOrName {
			return &issueTypes[i], nil
		}
	}

	return nil, nil
}

package issue_type

import "github.com/spf13/cobra"

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

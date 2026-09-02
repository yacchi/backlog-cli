package ratelimit

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

// RateLimitCmd は `backlog rate-limit` コマンド
var RateLimitCmd = &cobra.Command{
	Use:   "rate-limit",
	Short: "Show the current Backlog API rate limit status",
	Long: `Show the Backlog API rate limit status for the space of the currently
authenticated user.

Backlog does not use one global rate-limit bucket. Limits are tracked per
space and split into independent categories — read, update, search, and
icon — each with its own limit, remaining count, and reset time. Being
throttled on one category (e.g. a burst of "search" calls) does not mean
another category (e.g. "read") is also exhausted. When a request fails with
HTTP 429, the right response is to run this command, identify which
category is exhausted, and either switch to a request type in a category
that still has budget or wait until that category's reset time — not to
retry blindly, which will keep failing until the reset.

"reset" is a Unix timestamp (seconds since epoch). Table output renders it
as a local-time "YYYY-MM-DD HH:MM:SS" string for readability; JSON output
(--output json) leaves it as the raw number so it can be compared against
the current time or used to compute a wait duration.`,
	Example: `  # Check overall rate-limit status, e.g. before starting a bulk operation
  backlog rate-limit

  # Get the remaining "search" budget as a raw number, for use in a script
  backlog rate-limit --output json --jq '.search.remaining'

  # After a 429 on a search-heavy call, check when the search budget resets
  backlog rate-limit --output json --jq '.search.reset'`,
	RunE: runRateLimit,
}

func runRateLimit(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	rateLimit, err := client.GetRateLimit(c.Context())
	if err != nil {
		return fmt.Errorf("failed to get rate limit: %w", err)
	}

	profile := cfg.CurrentProfile()
	display := cfg.Display()

	switch profile.Output {
	case "json":
		return cmdutil.OutputJSONFromProfile(rateLimit, profile.JSONFields, profile.JQ, profile.Template)
	default:
		renderRateLimitTable(rateLimit, display.Timezone)
		return nil
	}
}

// rateLimitRow は表示用の1カテゴリ分の行
type rateLimitRow struct {
	Category string
	Value    api.RateLimitCategory
}

// rateLimitRows は RateLimit をカテゴリ名付きの行のスライスに変換する
// （表示順を固定するためのヘルパー）
func rateLimitRows(r *api.RateLimit) []rateLimitRow {
	return []rateLimitRow{
		{"read", r.Read},
		{"update", r.Update},
		{"search", r.Search},
		{"icon", r.Icon},
	}
}

// formatResetLocal は Unix タイムスタンプをローカルタイムゾーンの
// "YYYY-MM-DD HH:MM:SS" 形式に変換する
func formatResetLocal(reset int64, timezone string) string {
	loc := time.Local
	if timezone != "" {
		if l, err := time.LoadLocation(timezone); err == nil {
			loc = l
		}
	}
	return time.Unix(reset, 0).In(loc).Format("2006-01-02 15:04:05")
}

func renderRateLimitTable(r *api.RateLimit, timezone string) {
	table := ui.NewTable("CATEGORY", "LIMIT", "REMAINING", "RESET")
	for _, row := range rateLimitRows(r) {
		table.AddRow(
			row.Category,
			fmt.Sprintf("%d", row.Value.Limit),
			fmt.Sprintf("%d", row.Value.Remaining),
			formatResetLocal(row.Value.Reset, timezone),
		)
	}
	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}

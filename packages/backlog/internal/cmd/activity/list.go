package activity

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List user activities",
	Long: `List a user's recent activities across all accessible projects.

The activities API has no date-range arguments (only minId/maxId/count<=100),
so --since/--until are resolved on the client side by paginating with maxId
and filtering on each activity's "created" timestamp.

Activity types (--type) accept comma-separated semantic names:
  issue-create(1), issue-update(2), issue-comment(3), issue-delete(4),
  wiki-create(5), wiki-update(6), wiki-delete(7), file-add(8), file-update(9),
  file-delete(10), svn-commit(11), git-push(12), git-repo-create(13),
  issue-bulk-update(14), project-user-add(15), project-user-remove(16),
  notify-add(17), pr-add(18), pr-update(19), pr-comment(20), pr-delete(21),
  milestone-add(22), milestone-update(23), milestone-delete(24),
  group-project-add(25), group-project-remove(26)

Examples:
  # My recent issue activity in the last week
  backlog activity list --user @me \
    --type issue-create,issue-update,issue-comment \
    --since 2026-05-26 --until 2026-06-02 -o json

  # Default: my recent issue activity (latest 100)
  backlog activity list`,
	RunE: runList,
}

var (
	listUser  string
	listType  string
	listSince string
	listUntil string
	listLimit int
	listOrder string
)

func init() {
	listCmd.Flags().StringVarP(&listUser, "user", "u", "@me", "Target user (@me, user ID, userId, or display name)")
	listCmd.Flags().StringVarP(&listType, "type", "t", strings.Join(defaultActivityTypes, ","), "Activity types (comma-separated semantic names or IDs)")
	listCmd.Flags().StringVar(&listSince, "since", "", "Filter by created date since (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listUntil, "until", "", "Filter by created date until (YYYY-MM-DD)")
	listCmd.Flags().IntVarP(&listLimit, "limit", "L", 100, "Maximum number of activities to fetch (0 = all within range)")
	listCmd.Flags().StringVar(&listOrder, "order", "desc", "Sort order: asc or desc")
}

func runList(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}
	ctx := c.Context()

	if listOrder != "asc" && listOrder != "desc" {
		return fmt.Errorf("invalid order: %s (must be asc or desc)", listOrder)
	}

	userID, err := cmdutil.ResolveUserID(ctx, client, listUser)
	if err != nil {
		return err
	}

	typeIDs, err := ParseTypes(listType)
	if err != nil {
		return err
	}

	display := cfg.Display()
	loc := resolveLocation(display.Timezone)

	sinceT, untilT, hasSince, hasUntil, err := parseDateRange(listSince, listUntil, loc)
	if err != nil {
		return err
	}

	activities, err := fetchActivities(ctx, client, &fetchParams{
		userID:   userID,
		typeIDs:  typeIDs,
		limit:    listLimit,
		sinceT:   sinceT,
		untilT:   untilT,
		hasSince: hasSince,
		hasUntil: hasUntil,
	})
	if err != nil {
		return fmt.Errorf("failed to get activities: %w", err)
	}

	// 内部では maxId ページングのため常に desc 取得。asc 要求時は最後に反転する。
	if listOrder == "asc" {
		for i, j := 0, len(activities)-1; i < j; i, j = i+1, j-1 {
			activities[i], activities[j] = activities[j], activities[i]
		}
	}

	if len(activities) == 0 {
		fmt.Println("No activities found")
		return nil
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		return cmdutil.OutputJSONFromProfile(activities, profile.JSONFields, profile.JQ, profile.Template)
	default:
		outputTable(activities, display.Timezone, display.DateTimeFormat)
		return nil
	}
}

type fetchParams struct {
	userID   int
	typeIDs  []int
	limit    int
	sinceT   time.Time
	untilT   time.Time
	hasSince bool
	hasUntil bool
}

// fetchActivities は maxId ページング + created のクライアントフィルタで期間内のアクティビティを集める。
// activities API は order=desc（新しい順）で取得するため、created が since より前になった時点で打ち切れる。
func fetchActivities(ctx context.Context, client *api.Client, p *fetchParams) ([]backlog.Activity, error) {
	const batchSize = 100
	var result []backlog.Activity
	maxID := 0

	for {
		opts := &api.ActivityListOptions{
			UserID:          p.userID,
			ActivityTypeIDs: p.typeIDs,
			Count:           batchSize,
			Order:           "desc",
		}
		if maxID > 0 {
			opts.MaxID = maxID
		}

		batch, err := client.GetUserActivities(ctx, opts)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		stop := false
		for _, a := range batch {
			created, ok := parseActivityCreated(a)
			if p.hasSince && ok && created.Before(p.sinceT) {
				// desc 取得なのでこれ以降はすべて since より前
				stop = true
				break
			}
			if withinRange(created, ok, p.sinceT, p.untilT, p.hasSince, p.hasUntil) {
				result = append(result, a)
				if p.limit > 0 && len(result) >= p.limit {
					stop = true
					break
				}
			}
		}
		if stop {
			break
		}
		if len(batch) < batchSize {
			break
		}

		lastID := batch[len(batch)-1].ID.Value
		if lastID <= 0 {
			break
		}
		maxID = lastID - 1
	}

	return result, nil
}

// parseActivityCreated は activity の created を time.Time にパースする。
func parseActivityCreated(a backlog.Activity) (time.Time, bool) {
	if !a.Created.IsSet() || a.Created.Value == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, a.Created.Value)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// parseDateRange は YYYY-MM-DD の since/until を [since 00:00:00, until 23:59:59.999...] に変換する。
func parseDateRange(since, until string, loc *time.Location) (sinceT, untilT time.Time, hasSince, hasUntil bool, err error) {
	if since != "" {
		t, perr := time.ParseInLocation("2006-01-02", since, loc)
		if perr != nil {
			return sinceT, untilT, false, false, fmt.Errorf("invalid --since %q (expected YYYY-MM-DD)", since)
		}
		sinceT = t
		hasSince = true
	}
	if until != "" {
		t, perr := time.ParseInLocation("2006-01-02", until, loc)
		if perr != nil {
			return sinceT, untilT, false, false, fmt.Errorf("invalid --until %q (expected YYYY-MM-DD)", until)
		}
		// until は当日いっぱいを含める
		untilT = t.Add(24*time.Hour - time.Nanosecond)
		hasUntil = true
	}
	return sinceT, untilT, hasSince, hasUntil, nil
}

// withinRange は created が [since, until] に含まれるか判定する。
// created がパース不能（ok=false）の場合は、レンジ指定があれば除外、無ければ含める。
func withinRange(created time.Time, ok bool, sinceT, untilT time.Time, hasSince, hasUntil bool) bool {
	if !ok {
		return !hasSince && !hasUntil
	}
	if hasSince && created.Before(sinceT) {
		return false
	}
	if hasUntil && created.After(untilT) {
		return false
	}
	return true
}

func resolveLocation(timezone string) *time.Location {
	if timezone != "" {
		if loc, err := time.LoadLocation(timezone); err == nil {
			return loc
		}
	}
	return time.Local
}

func outputTable(activities []backlog.Activity, timezone, dateTimeFormat string) {
	formatter := ui.NewFieldFormatter(timezone, dateTimeFormat, nil)
	table := ui.NewTable("TYPE", "PROJECT", "ISSUE", "SUMMARY", "CREATED")

	for _, a := range activities {
		typeName := "-"
		if a.Type.IsSet() {
			typeName = TypeName(a.Type.Value)
		}

		projectKey := "-"
		if p, ok := a.Project.Get(); ok && p.ProjectKey.IsSet() {
			projectKey = p.ProjectKey.Value
		}

		issueRef := "-"
		summary := "-"
		if content, ok := a.Content.Get(); ok {
			if keyID, ok := content.KeyID.Get(); ok && projectKey != "-" {
				issueRef = fmt.Sprintf("%s-%d", projectKey, keyID)
			}
			if s, ok := content.Summary.Get(); ok && s != "" {
				summary = s
			}
		}

		created := "-"
		if a.Created.IsSet() {
			created = formatter.FormatDateTime(a.Created.Value, "created")
		}

		table.AddRow(typeName, projectKey, issueRef, summary, created)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}

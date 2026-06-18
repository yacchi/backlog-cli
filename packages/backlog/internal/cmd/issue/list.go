package issue

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/debug"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/summary"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List issues",
	Long: `List issues in a project.

Examples:
  # List open issues (default)
  backlog issue list
  backlog issue ls

  # Search across all accessible projects (ignores the default project)
  backlog issue list -p all --mine --state all

  # Limit to multiple projects (comma-separated)
  backlog issue list -p PROJ1,PROJ2 --mine

  # Filter by assignee
  backlog issue list --assignee @me
  backlog issue list --mine

  # Filter by state
  backlog issue list --state closed
  backlog issue list --state all

  # Search issues
  backlog issue list --search "bug fix"

  # Filter by issue type
  backlog issue list --type Bug

  # Sort by priority
  backlog issue list --sort priority --order asc

  # Filter by individual status / priority (name or ID, comma-separated)
  backlog issue list --status 処理中,完了 --priority 高

  # Filter by updated date range (YYYY-MM-DD)
  backlog issue list --state all --updated-since 2026-04-21 --updated-until 2026-04-28

  # Show only issues with attachments under a parent issue
  backlog issue list --parent PROJ-10 --has-attachment

  # Fetch up to 100 issues
  backlog issue list -L 100

  # Fetch all issues (auto-pagination)
  backlog issue list -L 0

  # Open issue list in browser
  backlog issue list --web

Available JSON fields (--json):
  id, issueKey, keyId, projectId, issueType, summary, description,
  resolution, priority, status, assignee, category, versions, milestone,
  startDate, dueDate, estimatedHours, actualHours, parentIssueId,
  createdUser, created, updatedUser, updated, customFields, attachments,
  sharedFiles, stars

Available table fields (default: key,status,priority,assignee,due_date,summary):
  key, status, priority, assignee, summary, type, created, updated,
  created_user, due_date, start_date, category, milestone, version, url`,
	RunE: runList,
}

var (
	listAssignee            string
	listAuthor              string
	listState               string
	listLimit               int
	listSearch              string
	listMine                bool
	listWeb                 bool
	listSummary             bool
	listSummaryWithComments bool
	listSummaryCommentCount int
	listMarkdown            bool
	listRaw                 bool
	listMarkdownWarn        bool
	listMarkdownCache       bool
	listCount               bool
	listCategory            string
	listMilestone           string
	listIssueType           string
	listSort                string
	listOrder               string
	listStatus              string
	listPriority            string
	listResolution          string
	listVersion             string
	listParent              string
	listParentChild         string
	listID                  string
	listHasAttachment       bool
	listHasSharedFile       bool
	listUpdatedSince        string
	listUpdatedUntil        string
	listCreatedSince        string
	listCreatedUntil        string
	listStartSince          string
	listStartUntil          string
	listDueSince            string
	listDueUntil            string
	listInvolved            string
	listIncludeCommented    bool
	listViewed              bool
	// gh-compatible aliases
	listSince   string
	listKeyword string
	listQuery   string
)

func init() {
	listCmd.Flags().StringVarP(&listAssignee, "assignee", "a", "", "Filter by assignee (user ID, userId, display name, or @me)")
	listCmd.Flags().StringVarP(&listAuthor, "author", "A", "", "Filter by author/creator (user ID, userId, display name, or @me)")
	listCmd.Flags().StringVarP(&listState, "state", "s", "open", "Filter by state: {open|closed|all}")
	listCmd.Flags().IntVarP(&listLimit, "limit", "L", 30, "Maximum number of issues to fetch")
	listCmd.Flags().StringVarP(&listSearch, "search", "S", "", "Search issues with keyword")
	listCmd.Flags().BoolVar(&listMine, "mine", false, "Show only my issues")
	listCmd.Flags().BoolVarP(&listWeb, "web", "w", false, "Open issue list in browser")
	listCmd.Flags().BoolVar(&listSummary, "summary", false, "Show AI summary column (description only)")
	listCmd.Flags().BoolVar(&listSummaryWithComments, "summary-with-comments", false, "Include comments in AI summary")
	listCmd.Flags().IntVar(&listSummaryCommentCount, "summary-comment-count", -1, "Number of comments to use for summary")
	listCmd.Flags().BoolVar(&listMarkdown, "markdown", false, "Render markdown by converting Backlog notation to GFM")
	listCmd.Flags().BoolVar(&listRaw, "raw", false, "Render raw content without markdown conversion")
	listCmd.Flags().BoolVar(&listMarkdownWarn, "markdown-warn", false, "Show markdown conversion warnings")
	listCmd.Flags().BoolVar(&listMarkdownCache, "markdown-cache", false, "Cache markdown conversion analysis data")
	listCmd.Flags().BoolVar(&listCount, "count", false, "Show only the count of issues")
	listCmd.Flags().StringVarP(&listCategory, "category", "l", "", "Filter by category IDs or names (comma-separated, like gh --label)")
	listCmd.Flags().StringVarP(&listMilestone, "milestone", "m", "", "Filter by milestone IDs or names (comma-separated)")
	listCmd.Flags().StringVarP(&listIssueType, "type", "T", "", "Filter by issue type IDs or names (e.g., 1, Bug, タスク)")
	listCmd.Flags().StringVar(&listSort, "sort", "updated", "Sort field: created, updated, issueType, category, priority, dueDate, etc.")
	listCmd.Flags().StringVar(&listOrder, "order", "desc", "Sort order: asc or desc")
	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by status IDs or names (comma-separated, e.g. 処理中,完了)")
	listCmd.Flags().StringVar(&listPriority, "priority", "", "Filter by priority IDs or names (comma-separated, e.g. 高)")
	listCmd.Flags().StringVar(&listResolution, "resolution", "", "Filter by resolution IDs or names (comma-separated)")
	listCmd.Flags().StringVar(&listVersion, "version", "", "Filter by affected version IDs or names (comma-separated)")
	listCmd.Flags().StringVar(&listParent, "parent", "", "Filter by parent issue IDs or keys (comma-separated)")
	listCmd.Flags().StringVar(&listParentChild, "parent-child", "", "Filter by parent-child relation: all, exclude-child, child, parent, none (or 0-4)")
	listCmd.Flags().StringVar(&listID, "id", "", "Filter by issue IDs or keys (comma-separated)")
	listCmd.Flags().BoolVar(&listHasAttachment, "has-attachment", false, "Show only issues with attachments")
	listCmd.Flags().BoolVar(&listHasSharedFile, "has-shared-file", false, "Show only issues with shared files")
	listCmd.Flags().StringVar(&listUpdatedSince, "updated-since", "", "Filter by updated date since (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listUpdatedUntil, "updated-until", "", "Filter by updated date until (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listCreatedSince, "created-since", "", "Filter by created date since (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listCreatedUntil, "created-until", "", "Filter by created date until (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listStartSince, "start-since", "", "Filter by start date since (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listStartUntil, "start-until", "", "Filter by start date until (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listDueSince, "due-since", "", "Filter by due date since (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listDueUntil, "due-until", "", "Filter by due date until (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listInvolved, "involved", "", "Show issues the user is involved in (assignee ∪ author); accepts @me, user ID, userId, or display name")
	listCmd.Flags().BoolVar(&listIncludeCommented, "include-commented", false, "With --involved, also scan comments to include comment-only involvement (slower, opt-in)")
	listCmd.Flags().BoolVar(&listViewed, "viewed", false, "Show recently viewed issues (opt-in; ignores other filters)")

	// gh-compatible aliases
	listCmd.Flags().StringVar(&listSince, "since", "", "Filter by created date since (YYYY-MM-DD) — alias for --created-since")
	listCmd.Flags().StringVar(&listKeyword, "keyword", "", "")
	listCmd.Flags().StringVarP(&listQuery, "query", "q", "", "")
	_ = listCmd.Flags().MarkHidden("keyword")
	_ = listCmd.Flags().MarkHidden("query")
}

// Backlog の標準ステータスID（全プロジェクト共通）
// 1=未対応, 2=処理中, 3=処理済み, 4=完了
var (
	standardOpenStatusIDs   = []int{1, 2, 3}
	standardClosedStatusIDs = []int{4}
)

// crossProjectKeyword は全プロジェクト横断検索を表す -p の特殊値。
// Backlog のプロジェクトキーは大文字固定のため、小文字 "all" は実プロジェクトと衝突しない。
const crossProjectKeyword = "all"

// parseProjectScope は -p/--project（またはデフォルト設定）の値を解析する。
//   - "all"（小文字単独）           -> 横断検索（projectKeys は空、crossProject=true）
//   - "INFRA,LCS" のようなカンマ区切り -> 複数プロジェクト
//   - 単一プロジェクトキー            -> 従来通り
//
// 横断でも複数でもなく、プロジェクトが何も指定されていない場合はエラーを返す。
func parseProjectScope(raw string) (projectKeys []string, crossProject bool, err error) {
	for _, part := range strings.Split(raw, ",") {
		key := strings.TrimSpace(part)
		if key == "" {
			continue
		}
		if key == crossProjectKeyword {
			crossProject = true
			continue
		}
		projectKeys = append(projectKeys, key)
	}

	if crossProject {
		if len(projectKeys) > 0 {
			return nil, false, fmt.Errorf("-p all (cross-project) cannot be combined with specific project keys")
		}
		return nil, true, nil
	}

	if len(projectKeys) == 0 {
		return nil, false, fmt.Errorf("project is required\nSpecify with -p/--project flag, use '-p all' to search across all projects, set in .backlog.yaml, or 'backlog config set profile.default.project <key>'")
	}

	return projectKeys, false, nil
}

func runList(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	profile := cfg.CurrentProfile()
	ctx := c.Context()

	// gh-compatible alias merging
	if err := mergeStringAlias(c, "since", &listSince, "created-since", &listCreatedSince); err != nil {
		return err
	}
	if err := mergeStringAlias(c, "keyword", &listKeyword, "search", &listSearch); err != nil {
		return err
	}
	if err := mergeStringAlias(c, "query", &listQuery, "search", &listSearch); err != nil {
		return err
	}

	// 日付フラグのバリデーション（YYYY-MM-DD形式のみ受け付ける）
	for _, df := range []struct{ name, value string }{
		{"--updated-since", listUpdatedSince},
		{"--updated-until", listUpdatedUntil},
		{"--created-since", listCreatedSince},
		{"--created-until", listCreatedUntil},
		{"--start-since", listStartSince},
		{"--start-until", listStartUntil},
		{"--due-since", listDueSince},
		{"--due-until", listDueUntil},
	} {
		if err := validateDateFlag(df.name, df.value); err != nil {
			return err
		}
	}

	// --involved / --viewed の併用ルール
	if listViewed && listInvolved != "" {
		return fmt.Errorf("--viewed cannot be combined with --involved")
	}
	if listInvolved != "" && (listMine || listAssignee != "" || listAuthor != "") {
		return fmt.Errorf("--involved cannot be combined with --mine/--assignee/--author (it already includes both assignee and author)")
	}
	if listIncludeCommented && listInvolved == "" {
		return fmt.Errorf("--include-commented requires --involved")
	}

	// --viewed は単独パス（プロジェクトや他フィルタを必要としない）
	if listViewed {
		issues, err := fetchViewedIssues(ctx, client)
		if err != nil {
			return err
		}
		if listCount {
			fmt.Println(len(issues))
			return nil
		}
		return renderIssueList(c, ctx, client, cfg, profile, issues, "")
	}

	// プロジェクト指定の解決
	//   - "all"（小文字、Backlog のプロジェクトキーは大文字固定のため衝突しない）: 全プロジェクト横断
	//   - "INFRA,LCS": 複数プロジェクト指定
	//   - 単一: 従来通り
	projectKeys, crossProject, err := parseProjectScope(cmdutil.GetCurrentProject(cfg))
	if err != nil {
		return err
	}

	// singleProjectKey はプロジェクト固有フィルタ（category 等）の解決に使う。
	// 横断/複数指定時は空となり、プロジェクト固有フィルタは利用できない。
	singleProjectKey := ""
	if len(projectKeys) == 1 {
		singleProjectKey = projectKeys[0]
	}

	// ブラウザで開く
	if listWeb {
		var url string
		if singleProjectKey != "" {
			url = fmt.Sprintf("https://%s/find/%s", profile.Space, singleProjectKey)
		} else {
			// 横断/複数指定はスペース全体の検索画面を開く
			url = fmt.Sprintf("https://%s/find", profile.Space)
		}
		return browser.OpenURL(url)
	}

	// プロジェクト固有フィルタが単一プロジェクトを要求することを保証する
	requireSingleProject := func(flag string) (string, error) {
		if singleProjectKey != "" {
			return singleProjectKey, nil
		}
		if crossProject {
			return "", fmt.Errorf("--%s cannot be used with cross-project search (-p all); specify a single project with -p/--project", flag)
		}
		return "", fmt.Errorf("--%s requires a single project; specify exactly one with -p/--project", flag)
	}

	// オプション構築
	opts := &api.IssueListOptions{
		Count: listLimit,
		Sort:  listSort,
		Order: listOrder,
	}

	// プロジェクトIDを解決（横断時は projectId[] を送らない）
	for _, key := range projectKeys {
		project, err := client.GetProject(ctx, key)
		if err != nil {
			return fmt.Errorf("failed to get project %q: %w", key, err)
		}
		opts.ProjectIDs = append(opts.ProjectIDs, project.ID)
	}

	if listSearch != "" {
		opts.Keyword = listSearch
	}

	// 担当者フィルター（@me・数値IDは横断でも解決可能、名前指定は単一プロジェクト必須）
	if listMine {
		assigneeID, err := cmdutil.ResolveProjectAssigneeID(ctx, client, singleProjectKey, "@me")
		if err != nil {
			return err
		}
		opts.AssigneeIDs = []int{assigneeID}
	} else if listAssignee != "" {
		assigneeID, err := cmdutil.ResolveProjectAssigneeID(ctx, client, singleProjectKey, listAssignee)
		if err != nil {
			return fmt.Errorf("failed to resolve assignee: %w", err)
		}
		opts.AssigneeIDs = []int{assigneeID}
	}

	// 作成者フィルター
	if listAuthor != "" {
		authorID, err := cmdutil.ResolveProjectAuthorID(ctx, client, singleProjectKey, listAuthor)
		if err != nil {
			return fmt.Errorf("failed to resolve author: %w", err)
		}
		opts.CreatedUserIDs = []int{authorID}
	}

	// カテゴリフィルター（--category オプション、ghの--labelに相当）
	if listCategory != "" {
		projectKey, err := requireSingleProject("category")
		if err != nil {
			return err
		}
		categoryIDs, err := cmdutil.ResolveCategoryIDs(ctx, client, projectKey, listCategory)
		if err != nil {
			return fmt.Errorf("failed to resolve categories: %w", err)
		}
		opts.CategoryIDs = categoryIDs
	}

	// マイルストーンフィルター（--milestone オプション）
	if listMilestone != "" {
		projectKey, err := requireSingleProject("milestone")
		if err != nil {
			return err
		}
		milestoneIDs, err := cmdutil.ResolveMilestoneIDs(ctx, client, projectKey, listMilestone)
		if err != nil {
			return fmt.Errorf("failed to resolve milestones: %w", err)
		}
		opts.MilestoneIDs = milestoneIDs
	}

	// 課題種別フィルター（--type オプション）
	if listIssueType != "" {
		projectKey, err := requireSingleProject("type")
		if err != nil {
			return err
		}
		issueTypeIDs, err := cmdutil.ResolveIssueTypeIDs(ctx, client, projectKey, listIssueType)
		if err != nil {
			return fmt.Errorf("failed to resolve issue types: %w", err)
		}
		opts.IssueTypeIDs = issueTypeIDs
	}

	// 優先度フィルター（--priority オプション）
	if listPriority != "" {
		priorityIDs, err := cmdutil.ResolvePriorityIDs(ctx, client, listPriority)
		if err != nil {
			return fmt.Errorf("failed to resolve priorities: %w", err)
		}
		opts.PriorityIDs = priorityIDs
	}

	// 完了理由フィルター（--resolution オプション）
	if listResolution != "" {
		resolutionIDs, err := cmdutil.ResolveResolutionIDs(ctx, client, listResolution)
		if err != nil {
			return fmt.Errorf("failed to resolve resolutions: %w", err)
		}
		opts.ResolutionIDs = resolutionIDs
	}

	// 発生バージョンフィルター（--version オプション）
	if listVersion != "" {
		projectKey, err := requireSingleProject("version")
		if err != nil {
			return err
		}
		versionIDs, err := cmdutil.ResolveVersionIDs(ctx, client, projectKey, listVersion)
		if err != nil {
			return fmt.Errorf("failed to resolve versions: %w", err)
		}
		opts.VersionIDs = versionIDs
	}

	// 親課題フィルター（--parent オプション）
	if listParent != "" {
		parentIDs, err := cmdutil.ResolveIssueIDs(ctx, client, listParent)
		if err != nil {
			return fmt.Errorf("failed to resolve parent issues: %w", err)
		}
		opts.ParentIssueIDs = parentIDs
	}

	// 親子条件フィルター（--parent-child オプション）
	if listParentChild != "" {
		parentChild, err := cmdutil.ParseParentChild(listParentChild)
		if err != nil {
			return err
		}
		opts.ParentChild = parentChild
	}

	// 課題IDフィルター（--id オプション）
	if listID != "" {
		issueIDs, err := cmdutil.ResolveIssueIDs(ctx, client, listID)
		if err != nil {
			return fmt.Errorf("failed to resolve issue ids: %w", err)
		}
		opts.IDs = issueIDs
	}

	// 添付・共有ファイルフィルター
	if listHasAttachment {
		hasAttachment := true
		opts.Attachment = &hasAttachment
	}
	if listHasSharedFile {
		hasSharedFile := true
		opts.SharedFile = &hasSharedFile
	}

	// 日付レンジフィルター
	opts.UpdatedSince = listUpdatedSince
	opts.UpdatedUntil = listUpdatedUntil
	opts.CreatedSince = listCreatedSince
	opts.CreatedUntil = listCreatedUntil
	opts.StartDateSince = listStartSince
	opts.StartDateUntil = listStartUntil
	opts.DueDateSince = listDueSince
	opts.DueDateUntil = listDueUntil

	// ステータスフィルター（--status オプション、--state より優先）
	if listStatus != "" {
		projectKey, err := requireSingleProject("status")
		if err != nil {
			return err
		}
		statusIDs, err := cmdutil.ResolveStatusIDs(ctx, client, projectKey, listStatus)
		if err != nil {
			return fmt.Errorf("failed to resolve statuses: %w", err)
		}
		opts.StatusIDs = statusIDs
	} else {
		// ステータスフィルター（--state オプション）
		switch listState {
		case "open":
			if singleProjectKey != "" {
				// 単一プロジェクトはカスタムステータスを考慮し、名前で判定する
				// Backlogの標準ステータス: 1=未対応, 2=処理中, 3=処理済み, 4=完了
				// open = 完了以外（4=完了を除く）
				statuses, err := client.GetStatuses(ctx, singleProjectKey)
				if err == nil {
					var openStatusIDs []int
					for _, s := range statuses {
						// "完了" または "Closed" 以外を含める
						if s.Name != "完了" && s.Name != "Closed" && s.Name != "Done" {
							openStatusIDs = append(openStatusIDs, s.ID)
						}
					}
					if len(openStatusIDs) > 0 {
						opts.StatusIDs = openStatusIDs
					}
				}
			} else {
				// 横断/複数指定はプロジェクト共通の標準ステータスIDで近似する
				// （プロジェクト固有のカスタムステータスは対象外）
				opts.StatusIDs = standardOpenStatusIDs
			}
		case "closed":
			if singleProjectKey != "" {
				// closed = 完了のみ
				statuses, err := client.GetStatuses(ctx, singleProjectKey)
				if err == nil {
					for _, s := range statuses {
						if s.Name == "完了" || s.Name == "Closed" || s.Name == "Done" {
							opts.StatusIDs = []int{s.ID}
							break
						}
					}
				}
			} else {
				opts.StatusIDs = standardClosedStatusIDs
			}
		case "all":
			// all = フィルターなし
		default:
			return fmt.Errorf("invalid state: %s (must be open, closed, or all)", listState)
		}
	}

	// 件数のみ表示（通常パスのみ。--involved は取得後に len で表示）
	if listCount && listInvolved == "" {
		count, err := client.GetIssuesCount(ctx, opts)
		if err != nil {
			return fmt.Errorf("failed to get issue count: %w", err)
		}
		fmt.Println(count)
		return nil
	}

	// 課題集合の決定
	var issues []backlog.Issue
	if listInvolved != "" {
		meID, err := cmdutil.ResolveUserID(ctx, client, listInvolved)
		if err != nil {
			return err
		}
		sinceT, untilT, hasSince, hasUntil, err := parseInvolvedRange(opts.UpdatedSince, opts.UpdatedUntil, cfg.Display().Timezone)
		if err != nil {
			return err
		}
		issues, err = resolveInvolvedIssues(ctx, client, opts, &involvedParams{
			meID:             meID,
			limit:            listLimit,
			includeCommented: listIncludeCommented,
			sinceT:           sinceT,
			untilT:           untilT,
			hasSince:         hasSince,
			hasUntil:         hasUntil,
		})
		if err != nil {
			return err
		}
	} else {
		issues, err = paginateIssues(ctx, client, opts, listLimit)
		if err != nil {
			return fmt.Errorf("failed to get issues: %w", err)
		}
	}

	if listCount {
		fmt.Println(len(issues))
		return nil
	}

	return renderIssueList(c, ctx, client, cfg, profile, issues, singleProjectKey)
}

// paginateIssues は opts に従って課題を取得する（limit 件まで、0 は全件）。
func paginateIssues(ctx context.Context, client *api.Client, opts *api.IssueListOptions, limit int) ([]backlog.Issue, error) {
	const batchSize = 100
	var allIssues []backlog.Issue
	remaining := limit
	if remaining == 0 {
		remaining = -1 // unlimited
	}

	for {
		fetchCount := batchSize
		if remaining > 0 && remaining < batchSize {
			fetchCount = remaining
		}
		opts.Count = fetchCount
		opts.Offset = len(allIssues)

		batch, err := client.GetIssues(ctx, opts)
		if err != nil {
			return nil, err
		}

		allIssues = append(allIssues, batch...)

		if len(batch) < fetchCount {
			break
		}
		if remaining > 0 {
			remaining -= len(batch)
			if remaining <= 0 {
				break
			}
		}
	}

	return allIssues, nil
}

// renderIssueList は課題リストを profile の出力形式（json/table）で出力する。
func renderIssueList(c *cobra.Command, ctx context.Context, client *api.Client, cfg *config.Store, profile *config.ResolvedProfile, issues []backlog.Issue, projectKey string) error {
	display := cfg.Display()
	switch profile.Output {
	case "json":
		return cmdutil.OutputJSONFromProfile(issues, profile.JSONFields, profile.JQ, profile.Template)
	default:
		if len(issues) == 0 {
			fmt.Println("No issues found")
			return nil
		}
		cacheDir, cacheErr := cfg.GetCacheDir()
		markdownOpts := cmdutil.ResolveMarkdownViewOptions(c, display, cacheDir)
		if markdownOpts.Cache && cacheErr != nil {
			return fmt.Errorf("failed to resolve cache dir: %w", cacheErr)
		}
		outputTable(ctx, client, issues, profile, display, cfg, projectKey, markdownOpts)
		return nil
	}
}

func outputTable(ctx context.Context, client *api.Client, issues []backlog.Issue, profile *config.ResolvedProfile, display *config.ResolvedDisplay, cfg *config.Store, projectKey string, markdownOpts cmdutil.MarkdownViewOptions) {
	// フラグ調整
	if listSummaryWithComments {
		listSummary = true
	}

	summaryCommentCount := display.SummaryCommentCount
	if listSummaryCommentCount >= 0 {
		summaryCommentCount = listSummaryCommentCount
	}

	// フィールドリストをコピーして操作
	fields := make([]string, len(display.IssueListFields))
	copy(fields, display.IssueListFields)

	if listSummary {
		fields = append(fields, "ai_summary")
	}

	fieldConfig := display.IssueFieldConfig

	// ハイパーリンク設定
	ui.SetHyperlinkEnabled(display.Hyperlink)

	// ヘッダー生成
	headers := make([]string, len(fields))
	for i, f := range fields {
		if f == "ai_summary" {
			headers[i] = "AI SUMMARY"
			continue
		}
		if cfg, ok := fieldConfig[f]; ok && cfg.Header != "" {
			headers[i] = cfg.Header
		} else {
			headers[i] = strings.ToUpper(f)
		}
	}

	table := ui.NewTable(headers...)

	// フィールドフォーマッターを作成
	formatter := ui.NewFieldFormatter(display.Timezone, display.DateTimeFormat, fieldConfig)

	// ベースURL生成
	baseURL := fmt.Sprintf("https://%s", profile.Space)

	// AI要約を一括取得
	summaryMap := make(map[string]string)
	if listSummary {
		summaryMap = fetchAISummaries(ctx, client, issues, cfg, summaryCommentCount, listSummaryWithComments, projectKey, baseURL, markdownOpts)
	}

	for _, issue := range issues {
		row := make([]string, len(fields))
		for i, f := range fields {
			row[i] = getIssueFieldValue(ctx, client, issue, f, formatter, baseURL, summaryMap, projectKey, markdownOpts)
		}
		table.AddRow(row...)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}

// fetchAISummaries はAI要約を一括取得する
func fetchAISummaries(ctx context.Context, client *api.Client, issues []backlog.Issue, cfg *config.Store, summaryCommentCount int, withComments bool, projectKey, baseURL string, markdownOpts cmdutil.MarkdownViewOptions) map[string]string {
	aiCfg := cfg.AISummary()
	if !aiCfg.Enabled {
		// AI要約が無効な場合は空のマップを返す
		fmt.Fprintln(os.Stderr, "Warning: AI summary is not enabled. Use 'backlog config set ai_summary.enabled true' to enable.")
		return make(map[string]string)
	}

	debug.Log("AI summary: starting",
		"provider", aiCfg.Provider,
		"issue_count", len(issues),
		"with_comments", withComments,
	)

	// Summarizerを作成
	summarizer, err := summary.NewSummarizer(aiCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: AI summary unavailable: %v\n", err)
		return make(map[string]string)
	}

	// 課題データを準備
	inputs := make([]summary.IssueInput, 0, len(issues))
	for _, issue := range issues {
		input := summary.IssueInput{
			Key:   issue.IssueKey.Value,
			Title: issue.Summary.Value,
		}

		if issue.Description.IsSet() {
			input.Description = issue.Description.Value
		}

		// コメント取得
		if withComments && summaryCommentCount > 0 {
			fetchCount := summaryCommentCount
			if fetchCount > 100 {
				fetchCount = 100
			}
			comments, err := client.GetComments(ctx, issue.IssueKey.Value, &api.CommentListOptions{
				Count: fetchCount,
				Order: "desc",
			})
			if err == nil {
				for i := len(comments) - 1; i >= 0; i-- {
					if comments[i].Content != "" {
						input.Comments = append(input.Comments, comments[i].Content)
					}
				}
			}
		}

		// Markdown変換
		if markdownOpts.Enable {
			issueID := 0
			if issue.ID.IsSet() {
				issueID = issue.ID.Value
			}
			issueKey := issue.IssueKey.Value
			issueURL := fmt.Sprintf("%s/view/%s", baseURL, issueKey)
			converted, err := cmdutil.RenderMarkdownContent(input.Description, markdownOpts, "issue", issueID, 0, projectKey, issueKey, issueURL, nil, nil)
			if err == nil {
				input.Description = converted
			}
		}

		inputs = append(inputs, input)
	}

	debug.Log("AI summary: prepared inputs",
		"input_count", len(inputs),
	)

	// 一括要約（進捗表示付き）
	stopProgress := ui.StartProgress("AI要約を生成中...")
	result, err := summarizer.SummarizeBatch(ctx, inputs)
	stopProgress()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: AI summary failed: %v\n", err)
		return make(map[string]string)
	}

	debug.Log("AI summary: completed",
		"result_count", len(result),
	)

	return result
}

func getIssueFieldValue(ctx context.Context, client *api.Client, issue backlog.Issue, field string, f *ui.FieldFormatter, baseURL string, summaryMap map[string]string, projectKey string, markdownOpts cmdutil.MarkdownViewOptions) string {
	switch field {
	case "key":
		key := issue.IssueKey.Value
		url := fmt.Sprintf("%s/view/%s", baseURL, key)
		return ui.Hyperlink(url, key)
	case "status":
		if issue.Status.IsSet() && issue.Status.Value.Name.IsSet() {
			return ui.StatusColor(issue.Status.Value.Name.Value)
		}
		return "-"
	case "priority":
		if issue.Priority.IsSet() && issue.Priority.Value.Name.IsSet() {
			return ui.PriorityColor(issue.Priority.Value.Name.Value)
		}
		return "-"
	case "assignee":
		if issue.Assignee.IsSet() && issue.Assignee.Value.Name.IsSet() {
			return f.FormatString(issue.Assignee.Value.Name.Value, field)
		}
		return "-"
	case "summary":
		return f.FormatString(issue.Summary.Value, field)
	case "ai_summary":
		// 事前に取得した要約マップから取得
		s, ok := summaryMap[issue.IssueKey.Value]
		if !ok || s == "" {
			return "-"
		}
		// 長すぎる場合は省略（テーブル表示のため）
		return summary.TruncateSummary(s, 50)
	case "type":
		if issue.IssueType.IsSet() && issue.IssueType.Value.Name.IsSet() {
			return issue.IssueType.Value.Name.Value
		}
		return "-"
	case "created":
		return f.FormatDateTime(issue.Created.Value, field)
	case "updated":
		return f.FormatDateTime(issue.Updated.Value, field)
	case "created_user":
		if issue.CreatedUser.IsSet() && issue.CreatedUser.Value.Name.IsSet() {
			return f.FormatString(issue.CreatedUser.Value.Name.Value, field)
		}
		return "-"
	case "due_date":
		if issue.DueDate.IsSet() && !issue.DueDate.IsNull() {
			return f.FormatDate(issue.DueDate.Value, field)
		}
		return "-"
	case "start_date":
		if issue.StartDate.IsSet() && !issue.StartDate.IsNull() {
			return f.FormatDate(issue.StartDate.Value, field)
		}
		return "-"
	case "category":
		if len(issue.Category) > 0 {
			names := make([]string, len(issue.Category))
			for i, c := range issue.Category {
				if c.Name.IsSet() {
					names[i] = c.Name.Value
				}
			}
			return f.FormatString(strings.Join(names, ", "), field)
		}
		return "-"
	case "milestone":
		if len(issue.Milestone) > 0 {
			names := make([]string, len(issue.Milestone))
			for i, m := range issue.Milestone {
				if m.Name.IsSet() {
					names[i] = m.Name.Value
				}
			}
			return f.FormatString(strings.Join(names, ", "), field)
		}
		return "-"
	case "version":
		if len(issue.Versions) > 0 {
			names := make([]string, len(issue.Versions))
			for i, v := range issue.Versions {
				if v.Name.IsSet() {
					names[i] = v.Name.Value
				}
			}
			return f.FormatString(strings.Join(names, ", "), field)
		}
		return "-"
	case "url":
		return fmt.Sprintf("%s/view/%s", baseURL, issue.IssueKey.Value)
	default:
		return "-"
	}
}

// mergeStringAlias copies the alias flag's value into the canonical flag when only the alias is set.
func mergeStringAlias(c *cobra.Command, aliasName string, aliasVar *string, canonicalName string, canonicalVar *string) error {
	if *aliasVar == "" {
		return nil
	}
	if c.Flags().Changed(canonicalName) {
		return fmt.Errorf("--%s and --%s cannot be used together", aliasName, canonicalName)
	}
	*canonicalVar = *aliasVar
	return nil
}

func validateDateFlag(name, value string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return fmt.Errorf("invalid %s %q: expected YYYY-MM-DD format (e.g. 2026-01-15)", name, value)
	}
	return nil
}

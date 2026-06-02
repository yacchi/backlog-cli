package issue

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

// involvedParams は --involved の取得パラメータ。
type involvedParams struct {
	meID             int
	limit            int
	includeCommented bool
	sinceT           time.Time
	untilT           time.Time
	hasSince         bool
	hasUntil         bool
}

// resolveInvolvedIssues は「関与課題」を課題ストアベースで横断取得する。
//
//	土台 = assignee ∪ author（2 コール + issueKey で union）
//	--include-commented 指定時は、union に出現したプロジェクト（または -p 明示時はそれ）を
//	候補に、コメントのみ関与の課題をクライアント側スキャンで追加する。
func resolveInvolvedIssues(ctx context.Context, client *api.Client, base *api.IssueListOptions, p *involvedParams) ([]backlog.Issue, error) {
	// assignee 軸
	assigneeOpts := *base
	assigneeOpts.AssigneeIDs = []int{p.meID}
	assigneeOpts.CreatedUserIDs = nil
	assigneeIssues, err := paginateIssues(ctx, client, &assigneeOpts, p.limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get assignee issues: %w", err)
	}

	// author 軸
	authorOpts := *base
	authorOpts.AssigneeIDs = nil
	authorOpts.CreatedUserIDs = []int{p.meID}
	authorIssues, err := paginateIssues(ctx, client, &authorOpts, p.limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get author issues: %w", err)
	}

	union := unionIssues(assigneeIssues, authorIssues)

	// コメントのみ関与のクライアント側スキャン（オプトイン）
	if p.includeCommented {
		commented, err := scanCommentedIssues(ctx, client, base, union, p)
		if err != nil {
			return nil, err
		}
		union = unionIssues(union, commented)
	}

	sortIssuesByUpdatedDesc(union)
	if p.limit > 0 && len(union) > p.limit {
		union = union[:p.limit]
	}
	return union, nil
}

// scanCommentedIssues は候補プロジェクトの課題を取得し、union 済みを差し引いた上で
// 各課題のコメントを調べ、自分が期間内にコメントした課題を返す。
func scanCommentedIssues(ctx context.Context, client *api.Client, base *api.IssueListOptions, existing []backlog.Issue, p *involvedParams) ([]backlog.Issue, error) {
	// 候補プロジェクト = -p 明示時はそれ、未指定（-p all）なら union 出現プロジェクト
	candidateOpts := *base
	candidateOpts.AssigneeIDs = nil
	candidateOpts.CreatedUserIDs = nil
	if len(base.ProjectIDs) == 0 {
		seen := make(map[int]struct{})
		var pids []int
		for _, is := range existing {
			pid := is.ProjectId.Value
			if pid <= 0 {
				continue
			}
			if _, ok := seen[pid]; ok {
				continue
			}
			seen[pid] = struct{}{}
			pids = append(pids, pid)
		}
		candidateOpts.ProjectIDs = pids
	}

	if len(candidateOpts.ProjectIDs) == 0 {
		// 候補プロジェクトが特定できない場合は全社スキャンを避けてスキップ
		return nil, nil
	}

	candidates, err := paginateIssues(ctx, client, &candidateOpts, p.limit)
	if err != nil {
		return nil, fmt.Errorf("failed to scan candidate issues: %w", err)
	}

	// union 済みを差し引く
	existKeys := make(map[string]struct{}, len(existing))
	for _, is := range existing {
		existKeys[is.IssueKey.Value] = struct{}{}
	}
	var toScan []backlog.Issue
	for _, is := range candidates {
		if _, ok := existKeys[is.IssueKey.Value]; ok {
			continue
		}
		toScan = append(toScan, is)
	}

	if len(toScan) == 0 {
		return nil, nil
	}

	stopProgress := ui.StartProgress(fmt.Sprintf("Scanning comments of %d issues...", len(toScan)))
	defer stopProgress()

	var matched []backlog.Issue
	for _, is := range toScan {
		ok, err := userCommentedInRange(ctx, client, is.IssueKey.Value, p)
		if err != nil {
			return nil, err
		}
		if ok {
			matched = append(matched, is)
		}
	}
	return matched, nil
}

// userCommentedInRange は指定課題に、自分が期間内に投稿したコメントが存在するか判定する。
// コメント API は desc（新しい順）で取得し、created が since より前になった時点で打ち切る。
func userCommentedInRange(ctx context.Context, client *api.Client, issueKey string, p *involvedParams) (bool, error) {
	const batchSize = 100
	maxID := 0
	for {
		opts := &api.CommentListOptions{Count: batchSize, Order: "desc"}
		if maxID > 0 {
			opts.MaxID = maxID
		}
		comments, err := client.GetComments(ctx, issueKey, opts)
		if err != nil {
			return false, err
		}
		if len(comments) == 0 {
			return false, nil
		}
		for _, cm := range comments {
			t, ok := parseRFC3339(cm.Created)
			if p.hasSince && ok && t.Before(p.sinceT) {
				// desc 取得なのでこれ以降はすべて since より前
				return false, nil
			}
			if cm.CreatedUser.ID == p.meID && timeInRange(t, ok, p.sinceT, p.untilT, p.hasSince, p.hasUntil) {
				return true, nil
			}
		}
		if len(comments) < batchSize {
			return false, nil
		}
		lastID := comments[len(comments)-1].ID
		if lastID <= 0 {
			return false, nil
		}
		maxID = lastID - 1
	}
}

// fetchViewedIssues は最近閲覧した課題一覧を取得する。
func fetchViewedIssues(ctx context.Context, client *api.Client) ([]backlog.Issue, error) {
	viewed, err := client.GetRecentlyViewedIssues(ctx, &api.RecentlyViewedIssuesOptions{Order: "desc", Count: 100})
	if err != nil {
		return nil, fmt.Errorf("failed to get recently viewed issues: %w", err)
	}
	issues := make([]backlog.Issue, 0, len(viewed))
	for _, v := range viewed {
		if is, ok := v.Issue.Get(); ok {
			issues = append(issues, is)
		}
	}
	return issues, nil
}

// unionIssues は複数の課題リストを issueKey で重複排除しつつ結合する（先勝ち）。
func unionIssues(lists ...[]backlog.Issue) []backlog.Issue {
	seen := make(map[string]struct{})
	var result []backlog.Issue
	for _, list := range lists {
		for _, is := range list {
			key := is.IssueKey.Value
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, is)
		}
	}
	return result
}

// sortIssuesByUpdatedDesc は updated 降順で安定ソートする（RFC3339 は辞書順 = 時系列順）。
func sortIssuesByUpdatedDesc(issues []backlog.Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].Updated.Value > issues[j].Updated.Value
	})
}

// parseInvolvedRange は YYYY-MM-DD の updated since/until を [since 00:00, until 23:59:59...] に変換する。
func parseInvolvedRange(since, until, timezone string) (sinceT, untilT time.Time, hasSince, hasUntil bool, err error) {
	loc := time.Local
	if timezone != "" {
		if l, lerr := time.LoadLocation(timezone); lerr == nil {
			loc = l
		}
	}
	if since != "" {
		t, perr := time.ParseInLocation("2006-01-02", since, loc)
		if perr != nil {
			return sinceT, untilT, false, false, fmt.Errorf("invalid --updated-since %q (expected YYYY-MM-DD)", since)
		}
		sinceT = t
		hasSince = true
	}
	if until != "" {
		t, perr := time.ParseInLocation("2006-01-02", until, loc)
		if perr != nil {
			return sinceT, untilT, false, false, fmt.Errorf("invalid --updated-until %q (expected YYYY-MM-DD)", until)
		}
		untilT = t.Add(24*time.Hour - time.Nanosecond)
		hasUntil = true
	}
	return sinceT, untilT, hasSince, hasUntil, nil
}

func parseRFC3339(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// timeInRange は t が [since, until] に含まれるか判定する。
// パース不能（ok=false）の場合、レンジ指定があれば除外、無ければ含める。
func timeInRange(t time.Time, ok bool, sinceT, untilT time.Time, hasSince, hasUntil bool) bool {
	if !ok {
		return !hasSince && !hasUntil
	}
	if hasSince && t.Before(sinceT) {
		return false
	}
	if hasUntil && t.After(untilT) {
		return false
	}
	return true
}

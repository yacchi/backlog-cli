package issue

import (
	"testing"
	"time"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
)

func issueWith(key, updated string) backlog.Issue {
	return backlog.Issue{
		IssueKey: backlog.NewOptString(key),
		Updated:  backlog.NewOptString(updated),
	}
}

func TestUnionIssues(t *testing.T) {
	assignee := []backlog.Issue{
		issueWith("PROJ-1", "2026-05-30T00:00:00Z"),
		issueWith("PROJ-2", "2026-05-29T00:00:00Z"),
	}
	author := []backlog.Issue{
		issueWith("PROJ-2", "2026-05-29T00:00:00Z"), // 重複
		issueWith("PROJ-3", "2026-05-31T00:00:00Z"), // author 純増
	}

	union := unionIssues(assignee, author)
	if len(union) != 3 {
		t.Fatalf("len(union) = %d, want 3", len(union))
	}

	keys := make(map[string]bool)
	for _, is := range union {
		if keys[is.IssueKey.Value] {
			t.Fatalf("duplicate key in union: %s", is.IssueKey.Value)
		}
		keys[is.IssueKey.Value] = true
	}
	for _, k := range []string{"PROJ-1", "PROJ-2", "PROJ-3"} {
		if !keys[k] {
			t.Fatalf("union missing %s", k)
		}
	}
}

func TestSortIssuesByUpdatedDesc(t *testing.T) {
	issues := []backlog.Issue{
		issueWith("PROJ-1", "2026-05-29T00:00:00Z"),
		issueWith("PROJ-2", "2026-05-31T00:00:00Z"),
		issueWith("PROJ-3", "2026-05-30T00:00:00Z"),
	}
	sortIssuesByUpdatedDesc(issues)
	want := []string{"PROJ-2", "PROJ-3", "PROJ-1"}
	for i, k := range want {
		if issues[i].IssueKey.Value != k {
			t.Fatalf("sorted[%d] = %s, want %s", i, issues[i].IssueKey.Value, k)
		}
	}
}

func TestTimeInRange(t *testing.T) {
	loc := time.UTC
	sinceT := time.Date(2026, 5, 26, 0, 0, 0, 0, loc)
	untilT := time.Date(2026, 6, 3, 0, 0, 0, 0, loc).Add(-time.Nanosecond)

	in := time.Date(2026, 5, 28, 0, 0, 0, 0, loc)
	before := time.Date(2026, 5, 20, 0, 0, 0, 0, loc)

	if !timeInRange(in, true, sinceT, untilT, true, true) {
		t.Fatal("in-range should be included")
	}
	if timeInRange(before, true, sinceT, untilT, true, true) {
		t.Fatal("before-since should be excluded")
	}
	if !timeInRange(before, true, time.Time{}, time.Time{}, false, false) {
		t.Fatal("no range should include all")
	}
}

func TestParseInvolvedRange(t *testing.T) {
	sinceT, untilT, hasSince, hasUntil, err := parseInvolvedRange("2026-05-26", "2026-06-02", "UTC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSince || !hasUntil {
		t.Fatalf("hasSince=%v hasUntil=%v", hasSince, hasUntil)
	}
	if !sinceT.Equal(time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("sinceT = %v", sinceT)
	}
	if !untilT.After(time.Date(2026, 6, 2, 23, 0, 0, 0, time.UTC)) {
		t.Fatalf("untilT = %v, want end of 2026-06-02", untilT)
	}

	if _, _, _, _, err := parseInvolvedRange("nope", "", "UTC"); err == nil {
		t.Fatal("expected error for invalid since")
	}
}

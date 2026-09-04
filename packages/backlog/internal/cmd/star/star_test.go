package star

import (
	"strings"
	"testing"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
)

func resetAddFlags() {
	addComment = 0
	addWiki = 0
	addPullRequest = 0
}

// runAdd/runRemove are exercised by calling the RunE functions directly
// rather than through Command.Execute(): addCmd/removeCmd are registered as
// children of StarCmd (see star.go's init()), so Execute() resolves the
// command tree from the root and would not test these commands in
// isolation. Calling RunE directly is the same code path cobra would
// invoke once argument parsing has completed.

func TestStarAddRejectsNoTarget(t *testing.T) {
	resetAddFlags()
	t.Cleanup(resetAddFlags)

	err := runAdd(addCmd, []string{})
	if err == nil {
		t.Fatal("expected error when no target is specified")
	}
	if !strings.Contains(err.Error(), "specify a target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStarAddRejectsMultipleTargetKinds(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		comment int
		wiki    int
		pr      int
	}{
		{name: "issue arg + --comment", args: []string{"PROJ-1"}, comment: 5},
		{name: "issue arg + --wiki", args: []string{"PROJ-1"}, wiki: 5},
		{name: "issue arg + --pull-request", args: []string{"PROJ-1"}, pr: 5},
		{name: "--comment + --wiki", args: []string{}, comment: 5, wiki: 6},
		{name: "--wiki + --pull-request", args: []string{}, wiki: 5, pr: 6},
		{name: "all three flags", args: []string{}, comment: 5, wiki: 6, pr: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetAddFlags()
			t.Cleanup(resetAddFlags)
			addComment = tt.comment
			addWiki = tt.wiki
			addPullRequest = tt.pr

			err := runAdd(addCmd, tt.args)
			if err == nil {
				t.Fatalf("expected usage error for %+v, got none (no API call should have been attempted)", tt)
			}
			if !strings.Contains(err.Error(), "exactly one star target") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestStarRemoveRejectsNonNumericID(t *testing.T) {
	err := runRemove(removeCmd, []string{"PROJ-1"})
	if err == nil {
		t.Fatal("expected error for non-numeric star ID")
	}
	if !strings.Contains(err.Error(), "invalid star ID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestStarRemoveDeclinesWithoutYesNonInteractively は、非対話環境で --yes を
// 付けずに実行した場合、確認プロンプトへの応答を待たずにエラーとなり、
// API クライアントの取得（延いては DELETE リクエスト）に到達しないことを確認する。
func TestStarRemoveDeclinesWithoutYesNonInteractively(t *testing.T) {
	t.Setenv("BACKLOG_ASSUME_YES", "")

	err := runRemove(removeCmd, []string{"75"})
	if err == nil {
		t.Fatal("expected error when confirmation is required but stdin is not interactive")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderStarTable(t *testing.T) {
	stars := []api.Star{
		{ID: 75, Title: "[PROJ-1] hello", URL: "https://x.backlog.jp/view/PROJ-1", Created: "2026-01-01T00:00:00Z"},
	}

	out := captureStdout(t, func() {
		renderStarTable(stars)
	})

	for _, want := range []string{"75", "[PROJ-1] hello", "https://x.backlog.jp/view/PROJ-1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table output missing %q: %s", want, out)
		}
	}
}

func TestStarListJSONOutputIncludesFields(t *testing.T) {
	stars := []api.Star{
		{ID: 75, Title: "[PROJ-1] hello", URL: "https://x.backlog.jp/view/PROJ-1", Created: "2026-01-01T00:00:00Z"},
	}

	out := captureStdout(t, func() {
		if err := cmdutil.OutputJSONFromProfile(stars, "", "", ""); err != nil {
			t.Fatalf("OutputJSONFromProfile returned error: %v", err)
		}
	})

	for _, want := range []string{`"id": 75`, `"title": "[PROJ-1] hello"`, `"url": "https://x.backlog.jp/view/PROJ-1"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("json output missing %q: %s", want, out)
		}
	}
}

func TestRenderStarTableEmpty(t *testing.T) {
	out := captureStdout(t, func() {
		renderStarTable(nil)
	})
	if !strings.Contains(out, "No stars found") {
		t.Fatalf("expected empty-state message, got: %s", out)
	}
}

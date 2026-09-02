package team

import (
	"strings"
	"testing"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
)

func TestRenderTeamTable(t *testing.T) {
	teams := []api.Team{
		{
			ID:      1,
			Name:    "team1",
			Members: []api.User{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Bob"}},
			Created: "2026-01-01T00:00:00Z",
		},
	}

	out := captureStdout(t, func() {
		renderTeamTable(teams)
	})

	for _, want := range []string{"1", "team1", "2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table output missing %q: %s", want, out)
		}
	}
}

func TestRenderTeamTableEmpty(t *testing.T) {
	out := captureStdout(t, func() {
		renderTeamTable(nil)
	})
	if !strings.Contains(out, "No teams found") {
		t.Fatalf("expected empty-state message, got: %s", out)
	}
}

func TestTeamListJSONOutputIncludesMemberCount(t *testing.T) {
	teams := []api.Team{
		{ID: 1, Name: "team1", Members: []api.User{{ID: 1, Name: "Alice"}}},
	}

	out := captureStdout(t, func() {
		if err := cmdutil.OutputJSONFromProfile(teams, "", "", ""); err != nil {
			t.Fatalf("OutputJSONFromProfile returned error: %v", err)
		}
	})

	for _, want := range []string{`"id": 1`, `"name": "team1"`, `"Alice"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("json output missing %q: %s", want, out)
		}
	}
}

func TestRenderTeamDetail(t *testing.T) {
	tm := &api.Team{
		ID:      5,
		Name:    "team5",
		Members: []api.User{{ID: 1, Name: "Alice", UserID: "alice"}},
	}

	out := captureStdout(t, func() {
		renderTeamDetail(tm)
	})

	for _, want := range []string{"team5", "Alice", "alice"} {
		if !strings.Contains(out, want) {
			t.Fatalf("detail output missing %q: %s", want, out)
		}
	}
}

// runView is called directly rather than through Command.Execute(): viewCmd
// is registered as a child of TeamCmd (see team.go's init()), so Execute()
// would resolve the command tree from the root instead of testing viewCmd
// in isolation.
func TestTeamViewRejectsNonNumericID(t *testing.T) {
	err := runView(viewCmd, []string{"not-a-number"})
	if err == nil {
		t.Fatal("expected error for non-numeric team ID")
	}
	if !strings.Contains(err.Error(), "invalid team ID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

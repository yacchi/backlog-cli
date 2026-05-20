package cmdutil

import (
	"strings"
	"testing"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
)

func TestResolveNamedIDs(t *testing.T) {
	options := []NamedResolverOption{
		{ID: 1, Label: "Bug"},
		{ID: 2, Label: "Task"},
		{ID: 3, Label: "Feature"},
	}

	ids, err := ResolveNamedIDs("Bug, 2,Feature", "issue type", "issue types", options)
	if err != nil {
		t.Fatalf("ResolveNamedIDs returned error: %v", err)
	}

	want := []int{1, 2, 3}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids[%d] = %d, want %d", i, ids[i], want[i])
		}
	}
}

func TestResolveNamedIDUsesAliases(t *testing.T) {
	options := []NamedResolverOption{
		{ID: 10, Label: "Alice", Aliases: []string{"alice"}, Description: "Alice (alice)"},
		{ID: 20, Label: "Bob", Aliases: []string{"bob"}, Description: "Bob (bob)"},
	}

	id, err := ResolveNamedID("ALICE", "assignee", "assignees", options)
	if err != nil {
		t.Fatalf("ResolveNamedID returned error: %v", err)
	}
	if id != 10 {
		t.Fatalf("id = %d, want %d", id, 10)
	}
}

func TestResolveNamedIDReportsAmbiguousMatches(t *testing.T) {
	options := []NamedResolverOption{
		{ID: 10, Label: "Alice", Aliases: []string{"alice-dev"}, Description: "Alice (alice-dev)"},
		{ID: 20, Label: "Alice", Aliases: []string{"alice-qa"}, Description: "Alice (alice-qa)"},
	}

	_, err := ResolveNamedID("Alice", "assignee", "assignees", options)
	if err == nil {
		t.Fatal("expected error")
	}

	msg := err.Error()
	for _, want := range []string{
		`multiple assignees match "Alice":`,
		`10 # Alice (alice-dev)`,
		`20 # Alice (alice-qa)`,
		`Use a numeric ID to disambiguate.`,
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error message missing %q:\n%s", want, msg)
		}
	}
}

func TestDescribeProjectUser(t *testing.T) {
	if got := describeProjectUser(api.User{ID: 10, UserID: "alice", Name: "Alice"}); got != "Alice (alice)" {
		t.Fatalf("describeProjectUser() = %q, want %q", got, "Alice (alice)")
	}
	if got := describeProjectUser(api.User{ID: 20, Name: "Bob"}); got != "Bob" {
		t.Fatalf("describeProjectUser() = %q, want %q", got, "Bob")
	}
}

func TestNonInteractiveFlagError(t *testing.T) {
	err := NonInteractiveFlagError(
		"--body is required when not running interactively",
		"backlog pr comment",
		"Use --body <text>.",
	)
	if err == nil {
		t.Fatal("expected error")
	}

	msg := err.Error()
	for _, want := range []string{
		"--body is required when not running interactively",
		"Use --body <text>.",
		"Run 'backlog pr comment --help' for usage.",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error message missing %q:\n%s", want, msg)
		}
	}
}

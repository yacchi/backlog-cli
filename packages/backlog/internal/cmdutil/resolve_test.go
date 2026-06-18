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

func TestResolveNamedIDPrefixMatch(t *testing.T) {
	options := []NamedResolverOption{
		{ID: 1, Label: "未対応"},
		{ID: 2, Label: "処理中"},
		{ID: 3, Label: "処理済み"},
		{ID: 4, Label: "完了"},
	}

	// "処理" matches both 処理中 and 処理済み → ambiguous
	_, err := ResolveNamedID("処理", "status", "statuses", options)
	if err == nil {
		t.Fatal("expected error for ambiguous prefix match")
	}
	if !strings.Contains(err.Error(), "Did you mean") {
		t.Fatalf("expected 'Did you mean' suggestion, got: %s", err.Error())
	}

	// "完" matches only 完了 → unique prefix match
	id, err := ResolveNamedID("完", "status", "statuses", options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 4 {
		t.Fatalf("id = %d, want 4", id)
	}
}

func TestResolveNamedIDContainsMatch(t *testing.T) {
	options := []NamedResolverOption{
		{ID: 1, Label: "初回リリース"},
		{ID: 2, Label: "環境クローズ"},
		{ID: 3, Label: "障害対応"},
	}

	// "障害" is contained only in 障害対応
	id, err := ResolveNamedID("障害", "issue type", "issue types", options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 3 {
		t.Fatalf("id = %d, want 3", id)
	}
}

func TestResolveNamedIDAliasWithFuzzy(t *testing.T) {
	options := []NamedResolverOption{
		{ID: 2, Label: "高", Aliases: []string{"high"}},
		{ID: 3, Label: "中", Aliases: []string{"medium", "normal"}},
		{ID: 4, Label: "低", Aliases: []string{"low"}},
	}

	tests := []struct {
		name  string
		input string
		want  int
	}{
		{name: "English exact", input: "high", want: 2},
		{name: "English uppercase", input: "HIGH", want: 2},
		{name: "English medium", input: "medium", want: 3},
		{name: "English normal", input: "normal", want: 3},
		{name: "English low", input: "Low", want: 4},
		{name: "Japanese exact", input: "高", want: 2},
		{name: "English prefix hi", input: "hi", want: 2},
		{name: "English prefix lo", input: "lo", want: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := ResolveNamedID(tt.input, "priority", "priorities", options)
			if err != nil {
				t.Fatalf("ResolveNamedID(%q) error: %v", tt.input, err)
			}
			if id != tt.want {
				t.Errorf("ResolveNamedID(%q) = %d, want %d", tt.input, id, tt.want)
			}
		})
	}
}

func TestResolveNamedIDIssueTypeAlias(t *testing.T) {
	options := []NamedResolverOption{
		{ID: 1, Label: "バグ", Aliases: []string{"bug"}},
		{ID: 2, Label: "タスク", Aliases: []string{"task"}},
		{ID: 3, Label: "要望", Aliases: []string{"feature", "enhancement", "request"}},
	}

	tests := []struct {
		input string
		want  int
	}{
		{"Bug", 1},
		{"bug", 1},
		{"BUG", 1},
		{"task", 2},
		{"Task", 2},
		{"feature", 3},
		{"enhancement", 3},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			id, err := ResolveNamedID(tt.input, "issue type", "issue types", options)
			if err != nil {
				t.Fatalf("ResolveNamedID(%q) error: %v", tt.input, err)
			}
			if id != tt.want {
				t.Errorf("ResolveNamedID(%q) = %d, want %d", tt.input, id, tt.want)
			}
		})
	}
}

func TestParseParentChild(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{name: "empty", input: "", want: 0},
		{name: "all", input: "all", want: 0},
		{name: "exclude-child", input: "exclude-child", want: 1},
		{name: "not-child alias", input: "not-child", want: 1},
		{name: "child", input: "child", want: 2},
		{name: "none", input: "none", want: 3},
		{name: "neither alias", input: "neither", want: 3},
		{name: "parent", input: "parent", want: 4},
		{name: "uppercase", input: "PARENT", want: 4},
		{name: "numeric 0", input: "0", want: 0},
		{name: "numeric 4", input: "4", want: 4},
		{name: "numeric with spaces", input: " 2 ", want: 2},
		{name: "numeric out of range", input: "5", wantErr: true},
		{name: "negative", input: "-1", wantErr: true},
		{name: "invalid string", input: "foo", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseParentChild(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseParentChild(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseParentChild(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseParentChild(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
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

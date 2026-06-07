package issue

import (
	"strings"
	"testing"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
)

func TestValidateNonInteractiveCreateFlags(t *testing.T) {
	err := validateNonInteractiveCreateFlags(
		createPromptState{},
		[]api.IssueType{
			{ID: 1, Name: "Bug"},
			{ID: 2, Name: "Task"},
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}

	msg := err.Error()
	for _, want := range []string{
		"--title, --type, and --priority required when not running interactively",
		"Use --title <text> to set the issue title.",
		"  --type Bug # ID: 1",
		"  --priority 3 # 中",
		"Run 'backlog issue create --help' for usage.",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error message missing %q:\n%s", want, msg)
		}
	}
}

func TestValidateNonInteractiveCreateFlags_AllowsExplicitInput(t *testing.T) {
	err := validateNonInteractiveCreateFlags(
		createPromptState{
			Title:    "title",
			Type:     "Bug",
			Priority: 3,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

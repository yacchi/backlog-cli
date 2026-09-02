package issue

import (
	"context"
	"errors"
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

func TestResolveParentIssueID_EmptyValueSendsNoParent(t *testing.T) {
	resolveCalled := false
	resolve := func(ctx context.Context, value string) ([]int, error) {
		resolveCalled = true
		return []int{999}, nil
	}

	id, err := resolveParentIssueID(context.Background(), resolve, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 0 {
		t.Fatalf("id = %d, want 0 (no parentIssueId field should be emitted)", id)
	}
	if resolveCalled {
		t.Fatal("resolver must not be called for an empty --parent value")
	}
}

func TestResolveParentIssueID_ResolvesKeyToNumericID(t *testing.T) {
	var gotValue string
	resolve := func(ctx context.Context, value string) ([]int, error) {
		gotValue = value
		return []int{42}, nil
	}

	id, err := resolveParentIssueID(context.Background(), resolve, "PROJ-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Fatalf("id = %d, want 42", id)
	}
	if gotValue != "PROJ-1" {
		t.Fatalf("resolver called with %q, want %q", gotValue, "PROJ-1")
	}
}

// TestResolveParentIssueID_ResolverErrorIsWrapped is an adversarial test derived
// from the repo preamble's coding rule "Wrap errors: fmt.Errorf(\"context: %w\",
// err)" and the contract's requirement that an unresolvable --parent value
// "exit with a clear error naming the value that could not be resolved". A
// resolver failure must propagate as a wrapped error the caller can unwrap
// with errors.Is, not be swallowed or replaced.
func TestResolveParentIssueID_ResolverErrorIsWrapped(t *testing.T) {
	sentinel := errors.New("boom")
	resolve := func(ctx context.Context, value string) ([]int, error) {
		return nil, sentinel
	}

	_, err := resolveParentIssueID(context.Background(), resolve, "PROJ-404")
	if err == nil {
		t.Fatal("expected error for a resolver failure")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("error %v does not wrap the underlying resolver error", err)
	}
	if !strings.Contains(err.Error(), "PROJ-404") {
		t.Fatalf("error %q does not name the unresolvable value", err.Error())
	}
}

func TestResolveParentIssueID_UnresolvableValueErrors(t *testing.T) {
	resolve := func(ctx context.Context, value string) ([]int, error) {
		return nil, errors.New("issue not found")
	}

	id, err := resolveParentIssueID(context.Background(), resolve, "NOPE-999")
	if err == nil {
		t.Fatal("expected error for an unresolvable --parent value")
	}
	if id != 0 {
		t.Fatalf("id = %d, want 0 on error", id)
	}
	if !strings.Contains(err.Error(), "NOPE-999") {
		t.Fatalf("error %q does not name the unresolvable value", err.Error())
	}
}

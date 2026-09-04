package issue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
)

// newRelatedRemoveTestCmd builds a bare *cobra.Command carrying the same
// persistent "-y/--yes" flag root.go registers, so shouldProceedWithRemoval
// can be exercised without constructing the real command tree.
func newRelatedRemoveTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "remove"}
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts (env: BACKLOG_ASSUME_YES)")
	return cmd
}

func failIfConfirmCalled(t *testing.T) func(string) (bool, error) {
	t.Helper()
	return func(message string) (bool, error) {
		t.Fatalf("confirmFn should not be called, got message: %q", message)
		return false, nil
	}
}

func relatedIssueFixture(key, status, summary string) api.RelatedIssue {
	return api.RelatedIssue{
		IssueKey: backlog.NewOptString(key),
		Status: backlog.NewOptStatus(backlog.Status{
			Name: backlog.NewOptString(status),
		}),
		Summary: backlog.NewOptString(summary),
		Type:    backlog.NewOptString("RELATES"),
	}
}

func TestRenderRelatedIssueList_Table(t *testing.T) {
	related := []api.RelatedIssue{
		relatedIssueFixture("PROJ-2", "処理中", "second issue"),
		relatedIssueFixture("PROJ-3", "完了", "third issue"),
	}

	var buf bytes.Buffer
	profile := &config.ResolvedProfile{}
	if err := renderRelatedIssueList(&buf, related, profile); err != nil {
		t.Fatalf("renderRelatedIssueList returned error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"KEY", "STATUS", "ASSIGNEE", "SUMMARY", "PROJ-2", "処理中", "second issue", "PROJ-3"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q, got:\n%s", want, out)
		}
	}
}

func TestRenderRelatedIssueList_NoResults(t *testing.T) {
	var buf bytes.Buffer
	profile := &config.ResolvedProfile{}
	if err := renderRelatedIssueList(&buf, nil, profile); err != nil {
		t.Fatalf("renderRelatedIssueList returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "No related issues found") {
		t.Errorf("expected empty-state message, got: %q", buf.String())
	}
}

func TestRenderRelatedIssueList_JSON(t *testing.T) {
	related := []api.RelatedIssue{
		relatedIssueFixture("PROJ-2", "処理中", "second issue"),
	}

	var buf bytes.Buffer
	profile := &config.ResolvedProfile{Output: "json"}
	if err := renderRelatedIssueList(&buf, related, profile); err != nil {
		t.Fatalf("renderRelatedIssueList returned error: %v", err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(decoded) != 1 {
		t.Fatalf("len(decoded) = %d, want 1", len(decoded))
	}
	if decoded[0]["issueKey"] != "PROJ-2" {
		t.Errorf("issueKey = %v, want PROJ-2", decoded[0]["issueKey"])
	}
}

// fakeRelatedIssueAdder records calls made to AddRelatedIssue and can be
// configured to fail on specific targets, without touching the network.
type fakeRelatedIssueAdder struct {
	calls   []int
	failIDs map[int]error
}

func (f *fakeRelatedIssueAdder) AddRelatedIssue(_ context.Context, issueIDOrKey string, relatedIssueID int) (*api.RelatedIssue, error) {
	f.calls = append(f.calls, relatedIssueID)
	if err, ok := f.failIDs[relatedIssueID]; ok {
		return nil, err
	}
	r := relatedIssueFixture(fmt.Sprintf("PROJ-%d", relatedIssueID), "未対応", "")
	return &r, nil
}

func atoiResolve(target string) (int, error) {
	return strconv.Atoi(target)
}

func TestAddRelatedIssueTargets_CallsAPIOncePerTarget(t *testing.T) {
	adder := &fakeRelatedIssueAdder{}
	var out bytes.Buffer

	results, failed := addRelatedIssueTargets(context.Background(), adder, "PROJ-1", []string{"2", "3"}, atoiResolve, &out)

	if len(adder.calls) != 2 {
		t.Fatalf("AddRelatedIssue called %d times, want 2", len(adder.calls))
	}
	if adder.calls[0] != 2 || adder.calls[1] != 3 {
		t.Errorf("calls = %v, want [2 3]", adder.calls)
	}
	if failed {
		t.Errorf("failed = true, want false")
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
}

func TestAddRelatedIssueTargets_ContinuesAfterFailureAndReportsFailure(t *testing.T) {
	adder := &fakeRelatedIssueAdder{
		failIDs: map[int]error{2: fmt.Errorf("failed to add related issue: boom")},
	}
	var out bytes.Buffer

	results, failed := addRelatedIssueTargets(context.Background(), adder, "PROJ-1", []string{"2", "3"}, atoiResolve, &out)

	if len(adder.calls) != 2 {
		t.Fatalf("AddRelatedIssue called %d times, want 2 (second target must still be attempted)", len(adder.calls))
	}
	if !failed {
		t.Errorf("failed = false, want true")
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1 (only the successful target)", len(results))
	}
	if !strings.Contains(out.String(), "Failed to add 2") {
		t.Errorf("expected failure report for target 2, got: %q", out.String())
	}
}

// fakeRelatedIssueRemover records calls made to DeleteRelatedIssue and can be
// configured to fail on specific targets, without touching the network.
type fakeRelatedIssueRemover struct {
	calls   []int
	failIDs map[int]error
}

func (f *fakeRelatedIssueRemover) DeleteRelatedIssue(_ context.Context, issueIDOrKey string, relatedIssueID int) (*api.RelatedIssue, error) {
	f.calls = append(f.calls, relatedIssueID)
	if err, ok := f.failIDs[relatedIssueID]; ok {
		return nil, err
	}
	r := relatedIssueFixture(fmt.Sprintf("PROJ-%d", relatedIssueID), "未対応", "")
	return &r, nil
}

func TestRemoveRelatedIssueTargets_CallsAPIOncePerTarget(t *testing.T) {
	remover := &fakeRelatedIssueRemover{}
	var out bytes.Buffer

	results, failed := removeRelatedIssueTargets(context.Background(), remover, "PROJ-1", []string{"2", "3"}, atoiResolve, &out)

	if len(remover.calls) != 2 {
		t.Fatalf("DeleteRelatedIssue called %d times, want 2", len(remover.calls))
	}
	if failed {
		t.Errorf("failed = true, want false")
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
}

// --- Adversarial additions (review pass) ---

// Contract C: "When the flag is not set, `issue view` must make exactly the
// same API calls it makes today ... no extra request" and relatedIssues is
// only included "under a relatedIssues key in the JSON output path" when
// --related is set. This checks the omission case directly against
// outputIssueJSON: relatedIssues=nil (flag absent) plus no comments must NOT
// go through the IssueWithComments envelope, so no "relatedIssues" (and no
// "comments") key appears at all.
func TestOutputIssueJSON_RelatedFlagAbsent_NoRelatedIssuesKey(t *testing.T) {
	issue := &backlog.Issue{Summary: backlog.NewOptString("plain issue")}
	profile := &config.ResolvedProfile{Output: "json"}

	// outputIssueJSON writes via cmdutil.OutputJSONFromProfile, which targets
	// os.Stdout directly (no io.Writer parameter) — capture it via a pipe.
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	jsonErr := outputIssueJSON(issue, nil, false, nil, profile)
	_ = w.Close()
	os.Stdout = origStdout
	if jsonErr != nil {
		t.Fatalf("outputIssueJSON returned error: %v", jsonErr)
	}
	captured, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(captured, &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, captured)
	}
	if _, ok := decoded["relatedIssues"]; ok {
		t.Errorf("relatedIssues key present when --related was not requested: %s", captured)
	}
	if _, ok := decoded["comments"]; ok {
		t.Errorf("comments key present when comments were not requested: %s", captured)
	}
	if _, ok := decoded["issue"]; ok {
		t.Errorf("issue got wrapped in an envelope even though neither comments nor related issues were requested: %s", captured)
	}
}

// Contract: "add"/"remove" that produce zero successful results (e.g. every
// target failed) must still serialize as a valid empty JSON array, not
// "null" and not an error — an agent parsing `--output json` should always
// get valid, parseable JSON.
func TestOutputRelatedIssuesJSON_EmptyResults_IsValidJSONArray(t *testing.T) {
	var buf bytes.Buffer
	profile := &config.ResolvedProfile{Output: "json"}
	if err := outputRelatedIssuesJSON(&buf, nil, profile); err != nil {
		t.Fatalf("outputRelatedIssuesJSON returned error for empty results: %v", err)
	}

	trimmed := strings.TrimSpace(buf.String())
	if trimmed == "null" {
		t.Fatalf("expected valid empty JSON array, got literal null: %q", trimmed)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(decoded) != 0 {
		t.Fatalf("len(decoded) = %d, want 0", len(decoded))
	}
}

// Contract: table rendering must not panic and must show a clear placeholder
// (not a zero-value crash) when the API omits optional fields (an
// unassigned issue has no `assignee`, and fields can in principle arrive
// unset). This is a boundary/omission case none of the worker's fixtures
// exercised (every fixture set all fields via relatedIssueFixture).
func TestRelatedIssueHelpers_UnsetOptionalFields(t *testing.T) {
	bare := api.RelatedIssue{
		IssueKey: backlog.NewOptString("PROJ-9"),
		Type:     backlog.NewOptString("RELATES"),
		// Status, Assignee, Summary intentionally left unset.
	}

	if got := relatedIssueStatus(bare); got != "-" {
		t.Errorf("relatedIssueStatus(unset) = %q, want %q", got, "-")
	}
	if got := relatedIssueAssignee(bare); got != "(unassigned)" {
		t.Errorf("relatedIssueAssignee(unset) = %q, want %q", got, "(unassigned)")
	}
	if got := relatedIssueSummary(bare); got != "" {
		t.Errorf("relatedIssueSummary(unset) = %q, want empty string", got)
	}
	if got := relatedIssueKey(bare); got != "PROJ-9" {
		t.Errorf("relatedIssueKey = %q, want PROJ-9", got)
	}
}

// --- shouldProceedWithRemoval: confirmation gating for "related remove" ---

func TestShouldProceedWithRemoval_YesFlagBypassesPrompt(t *testing.T) {
	cmd := newRelatedRemoveTestCmd()
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatalf("failed to set --yes: %v", err)
	}

	proceed, err := shouldProceedWithRemoval(cmd, "PROJ-1", []string{"PROJ-2"},
		func() bool { t.Fatal("isInteractive should not be called when --yes bypasses"); return false },
		failIfConfirmCalled(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !proceed {
		t.Error("proceed = false, want true (--yes should bypass confirmation)")
	}
}

func TestShouldProceedWithRemoval_AssumeYesEnvBypassesPrompt(t *testing.T) {
	t.Setenv("BACKLOG_ASSUME_YES", "1")
	cmd := newRelatedRemoveTestCmd()

	proceed, err := shouldProceedWithRemoval(cmd, "PROJ-1", []string{"PROJ-2"},
		func() bool {
			t.Fatal("isInteractive should not be called when BACKLOG_ASSUME_YES bypasses")
			return false
		},
		failIfConfirmCalled(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !proceed {
		t.Error("proceed = false, want true (BACKLOG_ASSUME_YES should bypass confirmation)")
	}
}

func TestShouldProceedWithRemoval_NonInteractiveWithoutYesErrors(t *testing.T) {
	cmd := newRelatedRemoveTestCmd()

	proceed, err := shouldProceedWithRemoval(cmd, "PROJ-1", []string{"PROJ-2"},
		func() bool { return false }, // not a terminal
		failIfConfirmCalled(t))
	if err == nil {
		t.Fatal("expected an error demanding --yes in non-interactive mode, got nil")
	}
	if proceed {
		t.Error("proceed = true, want false alongside the non-interactive error")
	}
}

func TestShouldProceedWithRemoval_DeclinedBlocksRemoval(t *testing.T) {
	cmd := newRelatedRemoveTestCmd()
	confirmCalls := 0

	proceed, err := shouldProceedWithRemoval(cmd, "PROJ-1", []string{"PROJ-2"},
		func() bool { return true }, // interactive
		func(message string) (bool, error) {
			confirmCalls++
			if !strings.Contains(message, "PROJ-1") || !strings.Contains(message, "PROJ-2") {
				t.Errorf("confirm message missing issue/target identifiers: %q", message)
			}
			return false, nil // user declines
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proceed {
		t.Error("proceed = true, want false when the user declines the confirmation prompt")
	}
	if confirmCalls != 1 {
		t.Fatalf("confirmFn called %d times, want 1", confirmCalls)
	}
}

func TestShouldProceedWithRemoval_ConfirmedAllowsRemoval(t *testing.T) {
	cmd := newRelatedRemoveTestCmd()

	proceed, err := shouldProceedWithRemoval(cmd, "PROJ-1", []string{"PROJ-2"},
		func() bool { return true },
		func(string) (bool, error) { return true, nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !proceed {
		t.Error("proceed = false, want true when the user confirms")
	}
}

func TestShouldProceedWithRemoval_ConfirmPromptErrorPropagates(t *testing.T) {
	cmd := newRelatedRemoveTestCmd()
	wantErr := errors.New("prompt io error")

	proceed, err := shouldProceedWithRemoval(cmd, "PROJ-1", []string{"PROJ-2"},
		func() bool { return true },
		func(string) (bool, error) { return false, wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if proceed {
		t.Error("proceed = true, want false when the confirm prompt itself errors")
	}
}

func TestRemoveRelatedIssueTargets_ContinuesAfterFailure(t *testing.T) {
	remover := &fakeRelatedIssueRemover{
		failIDs: map[int]error{2: fmt.Errorf("failed to delete related issue: boom")},
	}
	var out bytes.Buffer

	results, failed := removeRelatedIssueTargets(context.Background(), remover, "PROJ-1", []string{"2", "3"}, atoiResolve, &out)

	if len(remover.calls) != 2 {
		t.Fatalf("DeleteRelatedIssue called %d times, want 2 (second target must still be attempted)", len(remover.calls))
	}
	if !failed {
		t.Errorf("failed = false, want true")
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1 (only the successful target)", len(results))
	}
}

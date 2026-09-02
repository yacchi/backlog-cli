package issue

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var relatedCmd = &cobra.Command{
	Use:   "related <issue>",
	Short: "Show or manage an issue's related issues (\"see also\" links)",
	Long: `A "related issue" is a flat "see also" link between two issues (Backlog's
relation type "RELATES"). It carries no hierarchy: there is no parent/child,
no progress rollup, and no ordering between the two issues.

Without arguments, "backlog issue related <issue>" lists the issues linked to
<issue>. Use "add"/"remove" subcommands to change the links.

This is different from parent/child hierarchy. If you need a parent/child
relationship (with rollup semantics), use "backlog issue create --parent" or
"backlog issue edit --parent" instead of this command.

Examples:
  # List the issues related to PROJ-123
  backlog issue related PROJ-123

  # List related issues as JSON, for scripting/agent consumption
  backlog issue related PROJ-123 --output json

  # Extract only the related issue keys with jq
  backlog issue related PROJ-123 --jq '.[].issueKey'`,
	Args: cobra.ExactArgs(1),
	RunE: runRelatedList,
}

var relatedAddCmd = &cobra.Command{
	Use:   "add <issue> <related>...",
	Short: "Link one or more issues to <issue> as related issues",
	Long: `Add one or more "RELATES" links between <issue> and each <related> issue.
Each <related> value accepts either an issue key (e.g. PROJ-45) or a numeric
issue ID (e.g. 45).

Multiple <related> targets are applied one at a time, in order. If one target
fails (e.g. a bad key, or an issue you cannot access), the failure is reported
and the remaining targets are still attempted; the command exits non-zero if
any target failed.

This adds a flat "see also" link, not a parent/child relationship. For
parent/child hierarchy, use "backlog issue create --parent" or
"backlog issue edit --parent" instead.

Examples:
  # Link PROJ-123 to PROJ-45
  backlog issue related add PROJ-123 PROJ-45

  # Link PROJ-123 to several issues at once (numeric IDs also accepted)
  backlog issue related add PROJ-123 PROJ-45 PROJ-46 789

  # Add a link and confirm the result as JSON
  backlog issue related add PROJ-123 PROJ-45 --output json`,
	Args: cobra.MinimumNArgs(2),
	RunE: runRelatedAdd,
}

var relatedRemoveCmd = &cobra.Command{
	Use:     "remove <issue> <related>...",
	Aliases: []string{"rm"},
	Short:   "Remove the \"RELATES\" link between <issue> and one or more issues",
	Long: `Remove one or more "RELATES" links from <issue>. Each <related> value is the
identifier of the linked issue to unlink: an issue key (e.g. PROJ-45) or a
numeric issue ID (e.g. 45) — the same identifier you passed to "related add",
not a separate relation ID.

Multiple <related> targets are applied one at a time, in order. If one target
fails (e.g. the link does not exist), the failure is reported and the
remaining targets are still attempted; the command exits non-zero if any
target failed.

This is a destructive mutation: it prompts for confirmation unless -y/--yes
is set (or BACKLOG_ASSUME_YES is set in the environment).

Examples:
  # Remove the link between PROJ-123 and PROJ-45
  backlog issue related remove PROJ-123 PROJ-45

  # Remove several links at once, skipping the confirmation prompt
  backlog issue related remove PROJ-123 PROJ-45 PROJ-46 -y

  # Remove a link non-interactively (e.g. from a script or agent)
  backlog issue related remove PROJ-123 PROJ-45 --yes --output json`,
	Args: cobra.MinimumNArgs(2),
	RunE: runRelatedRemove,
}

func init() {
	relatedCmd.AddCommand(relatedAddCmd)
	relatedCmd.AddCommand(relatedRemoveCmd)
}

func runRelatedList(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	issueKey, _ := cmdutil.ResolveIssueKey(args[0], cmdutil.GetCurrentProject(cfg))

	related, err := client.GetRelatedIssues(c.Context(), issueKey)
	if err != nil {
		return fmt.Errorf("failed to get related issues: %w", err)
	}

	profile := cfg.CurrentProfile()
	return renderRelatedIssueList(c.OutOrStdout(), related, profile)
}

// renderRelatedIssueList renders related issues honoring the profile's
// output setting (table by default, JSON when profile.Output == "json",
// with -o/--output json, --json, --jq, -f/--format all folded into profile
// by the root command's PersistentPreRunE).
func renderRelatedIssueList(w io.Writer, related []api.RelatedIssue, profile *config.ResolvedProfile) error {
	switch profile.Output {
	case "json":
		return outputRelatedIssuesJSON(w, related, profile)
	default:
		if len(related) == 0 {
			_, _ = fmt.Fprintln(w, "No related issues found")
			return nil
		}
		renderRelatedIssueTable(w, related)
		return nil
	}
}

func outputRelatedIssuesJSON(w io.Writer, related []api.RelatedIssue, profile *config.ResolvedProfile) error {
	opts := cmdutil.JSONOutputOptions{Pretty: true}
	if profile.JSONFields != "" {
		opts.Fields = strings.Split(profile.JSONFields, ",")
	}
	if profile.Template != "" {
		opts.Template = profile.Template
	} else if profile.JQ != "" {
		opts.JQFilter = profile.JQ
	}
	return cmdutil.OutputJSON(w, related, opts)
}

func renderRelatedIssueTable(w io.Writer, related []api.RelatedIssue) {
	table := ui.NewTable("KEY", "STATUS", "ASSIGNEE", "SUMMARY")
	for _, r := range related {
		table.AddRow(
			relatedIssueKey(r),
			relatedIssueStatus(r),
			relatedIssueAssignee(r),
			relatedIssueSummary(r),
		)
	}
	table.RenderWithColor(w, ui.IsColorEnabled())
}

func relatedIssueKey(r api.RelatedIssue) string {
	if r.IssueKey.IsSet() {
		return r.IssueKey.Value
	}
	return "-"
}

func relatedIssueStatus(r api.RelatedIssue) string {
	if r.Status.IsSet() && r.Status.Value.Name.IsSet() {
		return r.Status.Value.Name.Value
	}
	return "-"
}

func relatedIssueAssignee(r api.RelatedIssue) string {
	if r.Assignee.IsSet() && !r.Assignee.Null && r.Assignee.Value.Name.IsSet() {
		return r.Assignee.Value.Name.Value
	}
	return "(unassigned)"
}

func relatedIssueSummary(r api.RelatedIssue) string {
	if r.Summary.IsSet() {
		return r.Summary.Value
	}
	return ""
}

// relatedIssueTarget resolves a single "add"/"remove" argument (an issue key
// or numeric issue ID) to a numeric issue ID via the existing key/id
// resolution helper (cmdutil.ResolveIssueIDs), reused here one target at a
// time so a failure on one target does not affect the others.
func relatedIssueTarget(ctx context.Context, client *api.Client, target string) (int, error) {
	ids, err := cmdutil.ResolveIssueIDs(ctx, client, target)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, fmt.Errorf("could not resolve %q to an issue", target)
	}
	return ids[0], nil
}

// relatedIssueAdder is the subset of *api.Client used by addRelatedIssueTargets,
// factored out so the continue-on-failure loop can be tested without a real
// API client.
type relatedIssueAdder interface {
	AddRelatedIssue(ctx context.Context, issueIDOrKey string, relatedIssueID int) (*api.RelatedIssue, error)
}

// addRelatedIssueTargets applies "related add" to each target in order,
// continuing past a per-target failure so the remaining targets are still
// attempted. It reports progress/failures to out and returns the successfully
// added relations plus whether any target failed.
func addRelatedIssueTargets(ctx context.Context, client relatedIssueAdder, issueKey string, targets []string, resolve func(target string) (int, error), out io.Writer) ([]api.RelatedIssue, bool) {
	var results []api.RelatedIssue
	var failed bool
	for _, target := range targets {
		id, err := resolve(target)
		if err != nil {
			failed = true
			_, _ = fmt.Fprintf(out, "Failed to add %s: %v\n", target, err)
			continue
		}
		added, err := client.AddRelatedIssue(ctx, issueKey, id)
		if err != nil {
			failed = true
			_, _ = fmt.Fprintf(out, "Failed to add %s: %v\n", target, err)
			continue
		}
		results = append(results, *added)
		ui.Success("Added related issue: %s", target)
	}
	return results, failed
}

func runRelatedAdd(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	issueKey, _ := cmdutil.ResolveIssueKey(args[0], cmdutil.GetCurrentProject(cfg))
	targets := args[1:]
	ctx := c.Context()
	out := c.OutOrStdout()

	resolve := func(target string) (int, error) {
		return relatedIssueTarget(ctx, client, target)
	}
	results, failed := addRelatedIssueTargets(ctx, client, issueKey, targets, resolve, out)

	profile := cfg.CurrentProfile()
	if profile.Output == "json" {
		if err := outputRelatedIssuesJSON(out, results, profile); err != nil {
			return err
		}
	}

	if failed {
		return fmt.Errorf("one or more related issues failed to add")
	}
	return nil
}

// relatedIssueRemover is the subset of *api.Client used by
// removeRelatedIssueTargets, factored out so the continue-on-failure loop
// can be tested without a real API client.
type relatedIssueRemover interface {
	DeleteRelatedIssue(ctx context.Context, issueIDOrKey string, relatedIssueID int) (*api.RelatedIssue, error)
}

// removeRelatedIssueTargets applies "related remove" to each target in
// order, continuing past a per-target failure so the remaining targets are
// still attempted. It reports progress/failures to out and returns the
// successfully removed relations plus whether any target failed.
func removeRelatedIssueTargets(ctx context.Context, client relatedIssueRemover, issueKey string, targets []string, resolve func(target string) (int, error), out io.Writer) ([]api.RelatedIssue, bool) {
	var results []api.RelatedIssue
	var failed bool
	for _, target := range targets {
		id, err := resolve(target)
		if err != nil {
			failed = true
			_, _ = fmt.Fprintf(out, "Failed to remove %s: %v\n", target, err)
			continue
		}
		removed, err := client.DeleteRelatedIssue(ctx, issueKey, id)
		if err != nil {
			failed = true
			_, _ = fmt.Fprintf(out, "Failed to remove %s: %v\n", target, err)
			continue
		}
		results = append(results, *removed)
		ui.Success("Removed related issue: %s", target)
	}
	return results, failed
}

// shouldProceedWithRemoval decides whether a "related remove" mutation should go ahead,
// applying the global -y/--yes / BACKLOG_ASSUME_YES bypass (cmdutil.SkipConfirmation) and,
// otherwise, an interactive confirmation prompt. It makes no config/API calls itself, so it
// can run — and be unit-tested — before any client is constructed, and the declined-confirmation
// path never reaches GetAPIClient.
//
// isInteractive and confirmFn are injected so every branch (bypassed, non-interactive-refused,
// confirmed, declined) is testable without a real terminal or survey prompt: production code
// passes ui.IsInteractiveInput and a function that shows an actual survey.Confirm prompt; tests
// pass fixed stubs.
func shouldProceedWithRemoval(c *cobra.Command, issueTarget string, targets []string, isInteractive func() bool, confirmFn func(message string) (bool, error)) (bool, error) {
	if cmdutil.SkipConfirmation(c) {
		return true, nil
	}
	if !isInteractive() {
		return false, cmdutil.NonInteractiveFlagError(
			"--yes is required when not running interactively",
			"backlog issue related remove",
			"Use --yes to skip the confirmation prompt.",
		)
	}
	message := fmt.Sprintf("Are you sure you want to remove the related issue link between %s and %s?", issueTarget, strings.Join(targets, ", "))
	return confirmFn(message)
}

func runRelatedRemove(c *cobra.Command, args []string) error {
	issueArg := args[0]
	targets := args[1:]
	out := c.OutOrStdout()

	proceed, err := shouldProceedWithRemoval(c, issueArg, targets, ui.IsInteractiveInput, func(message string) (bool, error) {
		var confirm bool
		if err := survey.AskOne(&survey.Confirm{Message: message, Default: false}, &confirm); err != nil {
			return false, err
		}
		return confirm, nil
	})
	if err != nil {
		return err
	}
	if !proceed {
		_, _ = fmt.Fprintln(out, "Aborted")
		return nil
	}

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	issueKey, _ := cmdutil.ResolveIssueKey(issueArg, cmdutil.GetCurrentProject(cfg))
	ctx := c.Context()

	resolve := func(target string) (int, error) {
		return relatedIssueTarget(ctx, client, target)
	}
	results, failed := removeRelatedIssueTargets(ctx, client, issueKey, targets, resolve, out)

	profile := cfg.CurrentProfile()
	if profile.Output == "json" {
		if err := outputRelatedIssuesJSON(out, results, profile); err != nil {
			return err
		}
	}

	if failed {
		return fmt.Errorf("one or more related issues failed to remove")
	}
	return nil
}

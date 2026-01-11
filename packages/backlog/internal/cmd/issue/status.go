package issue

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of relevant issues",
	Long: `Show status of relevant issues in the current project.

This command shows issues that are relevant to you:
- Issues assigned to you
- Issues created by you
- Recently updated issues

Examples:
  backlog issue status`,
	RunE: runStatus,
}

func init() {
	// No additional flags needed
}

func runStatus(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	profile := cfg.CurrentProfile()
	ctx := c.Context()

	// Get current user
	me, err := client.GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	// Get project
	project, err := client.GetProject(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// Get open status IDs
	statuses, err := client.GetStatuses(ctx, fmt.Sprintf("%d", project.ID))
	if err != nil {
		return fmt.Errorf("failed to get statuses: %w", err)
	}
	var openStatusIDs []int
	for _, s := range statuses {
		if s.Name != "完了" && s.Name != "Closed" && s.Name != "Done" {
			openStatusIDs = append(openStatusIDs, s.ID)
		}
	}

	// JSON output
	if profile.Output == "json" {
		result := struct {
			Assigned []backlog.Issue `json:"assigned"`
			Created  []backlog.Issue `json:"created"`
			Recent   []backlog.Issue `json:"recent"`
		}{}

		// Assigned to me
		assignedOpts := &api.IssueListOptions{
			ProjectIDs:  []int{project.ID},
			AssigneeIDs: []int{me.ID.Value},
			StatusIDs:   openStatusIDs,
			Count:       10,
			Sort:        "updated",
			Order:       "desc",
		}
		result.Assigned, _ = client.GetIssues(ctx, assignedOpts)

		// Created by me
		createdOpts := &api.IssueListOptions{
			ProjectIDs:     []int{project.ID},
			CreatedUserIDs: []int{me.ID.Value},
			StatusIDs:      openStatusIDs,
			Count:          10,
			Sort:           "updated",
			Order:          "desc",
		}
		result.Created, _ = client.GetIssues(ctx, createdOpts)

		// Recently updated
		recentOpts := &api.IssueListOptions{
			ProjectIDs: []int{project.ID},
			StatusIDs:  openStatusIDs,
			Count:      10,
			Sort:       "updated",
			Order:      "desc",
		}
		result.Recent, _ = client.GetIssues(ctx, recentOpts)

		return cmdutil.OutputJSONFromProfile(result, profile.JSONFields, profile.JQ)
	}

	// Table output
	baseURL := fmt.Sprintf("https://%s.%s", profile.Space, profile.Domain)

	// Assigned to you
	fmt.Printf("\n%s\n", ui.Bold("Assigned to you"))
	assignedOpts := &api.IssueListOptions{
		ProjectIDs:  []int{project.ID},
		AssigneeIDs: []int{me.ID.Value},
		StatusIDs:   openStatusIDs,
		Count:       10,
		Sort:        "updated",
		Order:       "desc",
	}
	assigned, err := client.GetIssues(ctx, assignedOpts)
	if err != nil {
		return fmt.Errorf("failed to get assigned issues: %w", err)
	}
	printStatusSection(assigned, baseURL)

	// Created by you
	fmt.Printf("\n%s\n", ui.Bold("Created by you"))
	createdOpts := &api.IssueListOptions{
		ProjectIDs:     []int{project.ID},
		CreatedUserIDs: []int{me.ID.Value},
		StatusIDs:      openStatusIDs,
		Count:          10,
		Sort:           "updated",
		Order:          "desc",
	}
	created, err := client.GetIssues(ctx, createdOpts)
	if err != nil {
		return fmt.Errorf("failed to get created issues: %w", err)
	}
	printStatusSection(created, baseURL)

	// Recently updated
	fmt.Printf("\n%s\n", ui.Bold("Recently updated"))
	recentOpts := &api.IssueListOptions{
		ProjectIDs: []int{project.ID},
		StatusIDs:  openStatusIDs,
		Count:      10,
		Sort:       "updated",
		Order:      "desc",
	}
	recent, err := client.GetIssues(ctx, recentOpts)
	if err != nil {
		return fmt.Errorf("failed to get recent issues: %w", err)
	}
	printStatusSection(recent, baseURL)

	fmt.Println()
	return nil
}

func printStatusSection(issues []backlog.Issue, baseURL string) {
	if len(issues) == 0 {
		fmt.Println("  No issues")
		return
	}

	table := ui.NewTable("", "KEY", "STATUS", "SUMMARY")
	for _, issue := range issues {
		key := issue.IssueKey.Value
		url := fmt.Sprintf("%s/view/%s", baseURL, key)

		status := "-"
		if issue.Status.IsSet() && issue.Status.Value.Name.IsSet() {
			status = ui.StatusColor(issue.Status.Value.Name.Value)
		}

		summary := issue.Summary.Value
		// Truncate summary if too long
		runes := []rune(summary)
		if len(runes) > 50 {
			summary = string(runes[:50]) + "..."
		}

		// Priority indicator
		priority := " "
		if issue.Priority.IsSet() && issue.Priority.Value.Name.IsSet() {
			switch issue.Priority.Value.Name.Value {
			case "高", "High":
				priority = ui.Red("!")
			case "低", "Low":
				priority = ui.Gray("○")
			}
		}

		table.AddRow(priority, ui.Hyperlink(url, key), status, summary)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}

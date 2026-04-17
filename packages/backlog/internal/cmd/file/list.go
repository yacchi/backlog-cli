package file

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list [<path>]",
	Short: "List project shared files",
	Long: `List shared files in the project. Defaults to the root directory.

Examples:
  backlog file list -p MYPROJ
  backlog file list -p MYPROJ /docs/design
  backlog file list -p MYPROJ --json id,name,type`,
	Args: cobra.MaximumNArgs(1),
	RunE: runList,
}

var listProject string

func init() {
	listCmd.Flags().StringVarP(&listProject, "project", "p", "", "Project key (required if not in project context)")
}

func runList(c *cobra.Command, args []string) error {
	dirPath := "/"
	if len(args) > 0 {
		dirPath = args[0]
	}

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	projectKey := listProject
	if projectKey == "" {
		if err := cmdutil.RequireProject(cfg); err != nil {
			return fmt.Errorf("project key required: use -p flag or run inside a project context")
		}
		projectKey = cmdutil.GetCurrentProject(cfg)
	}

	files, err := client.ListProjectFiles(c.Context(), projectKey, dirPath, &api.FileListOptions{Order: "asc"})
	if err != nil {
		return fmt.Errorf("failed to list files: %w", err)
	}

	profile := cfg.CurrentProfile()
	if err := cmdutil.OutputJSONFromProfile(files, profile.JSONFields, profile.JQ); err == nil {
		return nil
	}

	t := ui.NewTable()
	t.AddRow("ID", "TYPE", "NAME", "SIZE", "UPDATED")
	for _, f := range files {
		name := f.Name
		if f.Type == "directory" {
			name = name + "/"
		}
		size := ""
		if f.Type == "file" {
			size = formatBytes(f.Size)
		}
		t.AddRow(
			strconv.Itoa(f.ID),
			f.Type,
			name,
			size,
			shortDate(f.Updated),
		)
	}
	t.RenderWithColor(os.Stdout, ui.IsColorEnabled())
	return nil
}

func formatBytes(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case size >= GB:
		return fmt.Sprintf("%.1fGB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.1fMB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.1fKB", float64(size)/KB)
	default:
		return fmt.Sprintf("%dB", size)
	}
}

func shortDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

package repo

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var viewCmd = &cobra.Command{
	Use:   "view <repo-name>",
	Short: "View a Git repository",
	Long: `View details of a Git repository.

Examples:
  backlog repo view my-repo
  backlog repo view 123
  backlog repo view my-repo --project MYPROJECT`,
	Args: cobra.ExactArgs(1),
	RunE: runView,
}

func init() {
}

func runView(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	repoIDOrName := args[0]

	repo, err := client.GetGitRepository(c.Context(), projectKey, repoIDOrName)
	if err != nil {
		return fmt.Errorf("failed to get Git repository: %w", err)
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(repo)
	default:
		outputRepoDetail(repo)
		return nil
	}
}

func outputRepoDetail(repo *api.GitRepository) {
	// タイトル
	fmt.Printf("%s\n", ui.Bold(repo.Name))
	fmt.Println()

	// 詳細
	fmt.Printf("ID:          %d\n", repo.ID)

	if repo.Description != "" {
		fmt.Printf("Description: %s\n", repo.Description)
	}

	fmt.Println()
	fmt.Printf("HTTP URL:    %s\n", ui.Cyan(repo.HTTPURL))
	fmt.Printf("SSH URL:     %s\n", ui.Cyan(repo.SSHURL))

	if repo.PushedAt != "" {
		fmt.Printf("Last Push:   %s\n", repo.PushedAt)
	}

	fmt.Printf("Created:     %s\n", repo.Created)
	if repo.CreatedUser != nil {
		fmt.Printf("Created By:  %s\n", repo.CreatedUser.Name)
	}
}

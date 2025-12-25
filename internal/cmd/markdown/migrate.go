package markdown

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/backlog"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/markdown"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate Backlog notation to GFM",
	Long: `Migrate Backlog notation to GFM for a project.

This command suite supports init/apply/rollback steps to control API usage and
resume safely. Workspace data is stored in
the current directory unless --dir (or -w) is provided.

Examples:
  backlog markdown migrate init <projectKey>
  backlog markdown migrate apply
  backlog markdown migrate rollback
  backlog markdown migrate list
  backlog markdown migrate logs
  backlog markdown migrate status
  backlog markdown migrate clean
  backlog markdown migrate snapshot --append`,
}

var migrateInitCmd = &cobra.Command{
	Use:   "init <projectKey>",
	Short: "Initialize a migration workspace",
	Args:  cobra.ExactArgs(1),
	RunE:  runMigrateInit,
}

var (
	applyForceLock bool
	applyAuto      bool
	applyTypes     []string
	applyDryRun    bool
)

var migrateApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply converted data back to Backlog",
	Args:  cobra.NoArgs,
	RunE:  runMigrateApply,
}

var (
	rollbackForceLock bool
	rollbackTargets   []string
	rollbackTypes     []string
	rollbackAuto      bool
)

var migrateRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback applied conversions",
	Args:  cobra.NoArgs,
	RunE:  runMigrateRollback,
}

var migrateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List items in the migration workspace",
	Args:  cobra.NoArgs,
	RunE:  runMigrateList,
}

var migrateLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show migration logs",
	Args:  cobra.NoArgs,
	RunE:  runMigrateLogs,
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration workspace status",
	Args:  cobra.NoArgs,
	RunE:  runMigrateStatus,
}

var migrateCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove migration workspace",
	Args:  cobra.NoArgs,
	RunE:  runMigrateClean,
}

var migrateSnapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Snapshot migration data",
	Args:  cobra.NoArgs,
	RunE:  runMigrateSnapshot,
}

var listDiff bool
var migrateLogsLimit int
var migrateWorkspaceDir string
var migrateCleanForce bool
var migrateLogsAll bool
var snapshotAppend bool

func init() {
	migrateCmd.PersistentFlags().StringVarP(&migrateWorkspaceDir, "dir", "w", "", "Migration workspace directory (defaults to current directory)")
	migrateApplyCmd.Flags().BoolVar(&applyForceLock, "force-lock", false, "Remove existing lock and retry")
	migrateApplyCmd.Flags().BoolVar(&applyAuto, "auto", false, "Apply changes without confirmation")
	migrateApplyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Show diffs without applying changes")
	migrateApplyCmd.Flags().StringSliceVar(&applyTypes, "types", nil, "Apply target types (issue,wiki,issue_type). Default: all")
	migrateRollbackCmd.Flags().BoolVar(&rollbackForceLock, "force-lock", false, "Remove existing lock and retry")
	migrateRollbackCmd.Flags().BoolVar(&rollbackAuto, "auto", false, "Rollback without confirmation")
	migrateRollbackCmd.Flags().StringSliceVar(&rollbackTargets, "targets", nil, "Rollback target item keys (issue key, wiki id, issue type id)")
	migrateRollbackCmd.Flags().StringSliceVar(&rollbackTypes, "types", nil, "Rollback target types (issue,wiki,issue_type). Default: all")
	migrateListCmd.Flags().BoolVar(&listDiff, "diff", false, "Show diffs for changed items")
	migrateLogsCmd.Flags().IntVar(&migrateLogsLimit, "limit", 0, "Limit number of log entries (0 = all)")
	migrateLogsCmd.Flags().BoolVar(&migrateLogsAll, "all", false, "Include no-change entries")
	migrateCleanCmd.Flags().BoolVar(&migrateCleanForce, "force", false, "Remove workspace without confirmation")
	migrateSnapshotCmd.Flags().BoolVar(&snapshotAppend, "append", false, "Append new items to an existing workspace")

	migrateCmd.AddCommand(migrateInitCmd)
	migrateCmd.AddCommand(migrateApplyCmd)
	migrateCmd.AddCommand(migrateRollbackCmd)
	migrateCmd.AddCommand(migrateListCmd)
	migrateCmd.AddCommand(migrateLogsCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateCleanCmd)
	migrateCmd.AddCommand(migrateSnapshotCmd)
	MarkdownCmd.AddCommand(migrateCmd)
}

func runMigrateInit(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	projectKey := args[0]
	fmt.Printf("Initializing migration workspace for %s...\n", projectKey)
	project, err := client.GetProject(cmd.Context(), projectKey)
	if err != nil {
		return err
	}

	dir, err := migrationDir()
	if err != nil {
		return err
	}
	fmt.Printf("Workspace: %s\n", dir)

	if empty, err := isDirEmpty(dir); err != nil {
		return err
	} else if !empty {
		confirm := false
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Directory %s is not empty. Continue?", dir),
			Default: false,
		}
		if err := survey.AskOne(prompt, &confirm); err != nil {
			return err
		}
		if !confirm {
			fmt.Println("Canceled.")
			return nil
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create migration dir: %w", err)
	}

	metaPath := filepath.Join(dir, "metadata.json")
	if meta, err := loadMetadata(dir); err == nil {
		if meta.ProjectKey != "" && meta.ProjectKey != projectKey {
			return fmt.Errorf("workspace already initialized for %s", meta.ProjectKey)
		}
	}
	if _, err := os.Stat(metaPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat metadata: %w", err)
	}

	baseBranch, err := ensureMigrationRepo(dir, false)
	if err != nil {
		return err
	}

	if !gitHasCommits(dir) {
		fmt.Println("Creating initial commit...")
		if err := gitCommitAllowEmpty(dir, "init: workspace"); err != nil {
			return err
		}
	}

	if branch, err := gitCurrentBranch(dir); err == nil && branch != "" && branch != "HEAD" {
		baseBranch = branch
	}
	fmt.Printf("Base branch: %s\n", baseBranch)

	if err := ensureGitignore(dir); err != nil {
		return err
	}
	if err := ensureMetadata(dir, project.ProjectKey, project.Name, baseBranch); err != nil {
		return err
	}
	if err := gitAdd(dir, ".gitignore", "metadata.json"); err != nil {
		return err
	}
	if gitHasChanges(dir) {
		fmt.Println("Committing metadata...")
		if err := gitCommit(dir, "init: metadata"); err != nil {
			return err
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "items.jsonl")); err == nil {
		fmt.Printf("Workspace already initialized: %s\n", dir)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat items file: %w", err)
	}

	baseURL := fmt.Sprintf("https://%s.%s", cfg.CurrentProfile().Space, cfg.CurrentProfile().Domain)
	fmt.Printf("Snapshotting issues and wikis from %s...\n", baseURL)
	items, err := snapshotAll(cmd.Context(), client, projectKey, project.ID, dir, baseURL)
	if err != nil {
		return err
	}
	fmt.Printf("Snapshot complete: %d items\n", len(items))
	if err := writeItems(dir, items); err != nil {
		return err
	}
	fmt.Println("Writing items metadata...")
	if err := gitAdd(dir, "items.jsonl", "metadata.json", "issue", "wiki", "issue-type"); err != nil {
		return err
	}
	if gitHasChanges(dir) {
		fmt.Println("Committing initial snapshot...")
		if err := gitCommit(dir, "snapshot: init"); err != nil {
			return err
		}
	}
	fmt.Println("Init completed.")
	return nil
}

func runMigrateApply(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	dir, err := migrationDir()
	if err != nil {
		return err
	}
	meta, err := loadMetadata(dir)
	if err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}
	projectKey := meta.ProjectKey
	if projectKey == "" {
		return fmt.Errorf("metadata missing project key")
	}
	ctx := cmd.Context()
	unsafeRules := buildUnsafeRuleSet(cfg.Display().MarkdownUnsafeRules)

	baseBranch, err := ensureMigrationRepo(dir, true)
	if err != nil {
		return err
	}

	if meta.BaseBranch != "" {
		baseBranch = meta.BaseBranch
	}
	if resolved, err := resolveBaseBranch(dir, baseBranch); err != nil {
		return err
	} else if resolved != "" {
		baseBranch = resolved
	}

	currentBranch, err := gitCurrentBranch(dir)
	if err != nil {
		return err
	}
	startBranch := currentBranch
	applyBranch := currentBranch
	createdBranch := false
	if applyDryRun {
		if !strings.HasPrefix(currentBranch, "dry-run-") {
			applyBranch = fmt.Sprintf("dry-run-%s", time.Now().Format("20060102-150405"))
			if baseBranch != "" && gitBranchExists(dir, baseBranch) {
				if err := gitCheckout(dir, baseBranch); err != nil {
					return err
				}
				if err := gitCheckoutNewBranchFrom(dir, applyBranch, baseBranch); err != nil {
					return err
				}
			} else {
				if err := gitCheckoutNewBranch(dir, applyBranch); err != nil {
					return err
				}
			}
			createdBranch = true
		}
	} else if !strings.HasPrefix(currentBranch, "apply-") {
		if strings.HasPrefix(currentBranch, "dry-run-") && baseBranch != "" && gitBranchExists(dir, baseBranch) {
			if err := gitCheckout(dir, baseBranch); err != nil {
				return err
			}
		}
		applyBranch = fmt.Sprintf("apply-%s", time.Now().Format("20060102-150405"))
		if baseBranch != "" && gitBranchExists(dir, baseBranch) {
			if err := gitCheckoutNewBranchFrom(dir, applyBranch, baseBranch); err != nil {
				return err
			}
		} else if err := gitCheckoutNewBranch(dir, applyBranch); err != nil {
			return err
		}
		createdBranch = true
	}

	items, err := readItems(dir)
	if err != nil {
		return err
	}

	release, err := acquireLock(dir, applyForceLock)
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	applied := 0
	skipped := 0
	allowedTypes := normalizeTypes(applyTypes)
	anyChanges := false

	for i := range items {
		item := &items[i]
		if item.ItemType == "comment" {
			continue
		}
		if !typeAllowed(allowedTypes, item.ItemType) {
			continue
		}

		path, err := resolveItemPath(dir, item)
		if err != nil {
			return err
		}
		currentDisk, err := readFileIfExists(path)
		if err != nil {
			return fmt.Errorf("read content: %w", err)
		}

		current, err := fetchCurrentItem(ctx, client, item)
		if err != nil {
			errMsg := err.Error()
			_ = appendMigrateLog(dir, migrateLogEntry{
				Action:   "apply",
				Status:   "error",
				ItemType: item.ItemType,
				ItemKey:  item.ItemKey,
				URL:      item.URL,
				Message:  errMsg,
			})
			skipped++
			continue
		}

		nameChanged := item.ItemType == "wiki" && current.Name != "" && current.Name != item.ItemKey

		raw := current.Content
		if hashHex(raw) != item.InputHash {
			if err := writeItemContent(path, raw); err != nil {
				return err
			}
			item.InputHash = hashHex(raw)
			item.UpdatedAt = current.Updated
			if nameChanged {
				item.ItemKey = current.Name
				nameChanged = false
			}
			item.FetchedAt = time.Now().Format(time.RFC3339)
			item.Path = path
			if item.ItemType == "wiki" {
				if err := writeWikiMetadata(dir, item.ItemID, item.ItemKey, item.URL, item.UpdatedAt); err != nil {
					return err
				}
			}
			if err := writeItems(dir, items); err != nil {
				return err
			}
			addPaths := []string{path, "items.jsonl"}
			if item.ItemType == "wiki" {
				addPaths = append(addPaths, filepath.Join("wiki", fmt.Sprintf("%d", item.ItemID), "metadata.json"))
			}
			if err := gitAdd(dir, addPaths...); err != nil {
				return err
			}
			if gitHasChanges(dir) {
				if err := gitCommit(dir, fmt.Sprintf("snapshot: %s %s", item.ItemType, item.ItemKey)); err != nil {
					return err
				}
				anyChanges = true
			}
			currentDisk = raw
		} else if nameChanged {
			item.ItemKey = current.Name
			item.UpdatedAt = current.Updated
			if item.ItemType == "wiki" {
				if err := writeWikiMetadata(dir, item.ItemID, item.ItemKey, item.URL, item.UpdatedAt); err != nil {
					return err
				}
			}
			if err := writeItems(dir, items); err != nil {
				return err
			}
			addPaths := []string{"items.jsonl"}
			if item.ItemType == "wiki" {
				addPaths = append(addPaths, filepath.Join("wiki", fmt.Sprintf("%d", item.ItemID), "metadata.json"))
			}
			if err := gitAdd(dir, addPaths...); err != nil {
				return err
			}
			if gitHasChanges(dir) {
				if err := gitCommit(dir, fmt.Sprintf("snapshot: %s %s", item.ItemType, item.ItemKey)); err != nil {
					return err
				}
				anyChanges = true
			}
		}

		converted, changed, err := applyConversion(item, raw, current.Attachments, unsafeRules)
		if err != nil {
			errMsg := err.Error()
			_ = appendMigrateLog(dir, migrateLogEntry{
				Action:   "apply",
				Status:   "error",
				ItemType: item.ItemType,
				ItemKey:  item.ItemKey,
				URL:      item.URL,
				Message:  errMsg,
			})
			skipped++
			continue
		}
		if !changed || converted == currentDisk {
			_ = appendMigrateLog(dir, migrateLogEntry{
				Action:   "apply",
				Status:   "no_change",
				ItemType: item.ItemType,
				ItemKey:  item.ItemKey,
				URL:      item.URL,
			})
			skipped++
			continue
		}

		if !applyAuto {
			if err := printContentDiff(currentDisk, converted); err != nil {
				return err
			}
			if item.URL != "" {
				fmt.Printf("%s\n", item.URL)
			}
			fmt.Printf("\n%s %s\n", item.ItemType, item.ItemKey)
			choice, err := promptApplyDecision()
			if err != nil {
				return err
			}
			switch choice {
			case "approve":
			case "reject":
				_ = appendMigrateLog(dir, migrateLogEntry{
					Action:   "apply",
					Status:   "rejected",
					ItemType: item.ItemType,
					ItemKey:  item.ItemKey,
					URL:      item.URL,
				})
				skipped++
				continue
			case "skip":
				_ = appendMigrateLog(dir, migrateLogEntry{
					Action:   "apply",
					Status:   "skipped",
					ItemType: item.ItemType,
					ItemKey:  item.ItemKey,
					URL:      item.URL,
				})
				skipped++
				continue
			case "quit":
				if anyChanges {
					if err := finalizeApplyState(dir, items); err != nil {
						return err
					}
				}
				fmt.Println("Stopped by user.")
				return nil
			}
		}

		if applyDryRun && applyAuto {
			if err := printContentDiff(currentDisk, converted); err != nil {
				return err
			}
			if item.URL != "" {
				fmt.Printf("%s\n", item.URL)
			}
			fmt.Printf("\n%s %s\n", item.ItemType, item.ItemKey)
		}

		if !applyDryRun {
			updatedAt, err := applyItem(ctx, client, item, converted)
			if err != nil {
				errMsg := err.Error()
				if !applyAuto {
					fmt.Printf("Failed %s %s: %s\n", item.ItemType, item.ItemKey, errMsg)
				}
				_ = appendMigrateLog(dir, migrateLogEntry{
					Action:   "apply",
					Status:   "error",
					ItemType: item.ItemType,
					ItemKey:  item.ItemKey,
					URL:      item.URL,
					Message:  errMsg,
				})
				skipped++
				continue
			}
			if updatedAt != "" {
				item.UpdatedAt = updatedAt
			}
			item.InputHash = hashHex(converted)
			item.FetchedAt = time.Now().Format(time.RFC3339)
		}

		if err := writeItemContent(path, converted); err != nil {
			return err
		}
		item.OutputHash = hashHex(converted)
		if applyDryRun {
			item.Applied = false
			item.AppliedAt = ""
		} else {
			item.Applied = true
			item.AppliedAt = time.Now().Format(time.RFC3339)
		}
		item.ApplyError = ""
		if err := writeItems(dir, items); err != nil {
			return err
		}
		if err := gitAdd(dir, path, "items.jsonl"); err != nil {
			return err
		}
		if gitHasChanges(dir) {
			if err := gitCommit(dir, fmt.Sprintf("apply: %s %s", item.ItemType, item.ItemKey)); err != nil {
				return err
			}
			anyChanges = true
		}
		status := "applied"
		if applyDryRun {
			status = "dry_run"
		} else {
			applied++
		}
		_ = appendMigrateLog(dir, migrateLogEntry{
			Action:   "apply",
			Status:   status,
			ItemType: item.ItemType,
			ItemKey:  item.ItemKey,
			URL:      item.URL,
		})
		if applyDryRun {
			skipped++
		}
	}

	if anyChanges {
		if err := finalizeApplyState(dir, items); err != nil {
			return err
		}
	} else {
		fmt.Println("No changes; skipping merge and cleaning up.")
	}

	if anyChanges && baseBranch != "" && baseBranch != applyBranch && strings.HasPrefix(applyBranch, "apply-") && !applyDryRun {
		if !checkoutIfPossible(dir, baseBranch) {
			fmt.Printf("Base branch %s not available; skipping merge.\n", baseBranch)
			goto checkout_return
		}
		if err := gitMergeNoFF(dir, applyBranch); err != nil {
			return err
		}
	}
checkout_return:
	if !applyDryRun {
		if !checkoutIfPossible(dir, startBranch) {
			_ = checkoutIfPossible(dir, baseBranch)
		}
		if !anyChanges && strings.HasPrefix(applyBranch, "apply-") {
			_ = gitDeleteBranch(dir, applyBranch)
		}
	}

	if createdBranch {
		fmt.Printf("Applied: %d, Skipped: %d (branch %s)\n", applied, skipped, applyBranch)
		return nil
	}

	fmt.Printf("Applied: %d, Skipped: %d\n", applied, skipped)
	return nil
}

func runMigrateRollback(cmd *cobra.Command, args []string) error {
	client, _, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	dir, err := migrationDir()
	if err != nil {
		return err
	}
	meta, err := loadMetadata(dir)
	if err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}
	if meta.ProjectKey == "" {
		return fmt.Errorf("metadata missing project key")
	}

	if _, err := ensureMigrationRepo(dir, true); err != nil {
		return err
	}

	items, err := readItems(dir)
	if err != nil {
		return err
	}

	release, err := acquireLock(dir, rollbackForceLock)
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	rolledBack := 0
	skipped := 0
	allowedTypes := normalizeTypes(rollbackTypes)
	targets := normalizeTargets(rollbackTargets)
	anyChanges := false

	for i := range items {
		item := &items[i]
		if item.ItemType == "comment" {
			continue
		}
		if !typeAllowed(allowedTypes, item.ItemType) {
			continue
		}
		if len(targets) > 0 && !matchesRollbackTarget(item, targets) {
			continue
		}

		path, err := resolveItemPath(dir, item)
		if err != nil {
			return err
		}

		firstCommit, err := gitFirstCommitForFile(dir, path)
		if err != nil {
			errMsg := err.Error()
			_ = appendMigrateLog(dir, migrateLogEntry{
				Action:   "rollback",
				Status:   "error",
				ItemType: item.ItemType,
				ItemKey:  item.ItemKey,
				URL:      item.URL,
				Message:  errMsg,
			})
			skipped++
			continue
		}
		if firstCommit == "" {
			_ = appendMigrateLog(dir, migrateLogEntry{
				Action:   "rollback",
				Status:   "no_snapshot",
				ItemType: item.ItemType,
				ItemKey:  item.ItemKey,
				URL:      item.URL,
			})
			skipped++
			continue
		}

		if err := gitCheckoutFile(dir, firstCommit, path); err != nil {
			errMsg := err.Error()
			_ = appendMigrateLog(dir, migrateLogEntry{
				Action:   "rollback",
				Status:   "error",
				ItemType: item.ItemType,
				ItemKey:  item.ItemKey,
				URL:      item.URL,
				Message:  errMsg,
			})
			skipped++
			continue
		}

		content, err := readFileIfExists(path)
		if err != nil {
			errMsg := err.Error()
			_ = appendMigrateLog(dir, migrateLogEntry{
				Action:   "rollback",
				Status:   "error",
				ItemType: item.ItemType,
				ItemKey:  item.ItemKey,
				URL:      item.URL,
				Message:  errMsg,
			})
			skipped++
			continue
		}

		if !rollbackAuto {
			current, err := fetchCurrentItem(cmd.Context(), client, item)
			if err != nil {
				errMsg := err.Error()
				_ = appendMigrateLog(dir, migrateLogEntry{
					Action:   "rollback",
					Status:   "error",
					ItemType: item.ItemType,
					ItemKey:  item.ItemKey,
					URL:      item.URL,
					Message:  errMsg,
				})
				skipped++
				continue
			}
			if err := printContentDiff(current.Content, content); err != nil {
				return err
			}
			if item.URL != "" {
				fmt.Printf("%s\n", item.URL)
			}
			fmt.Printf("\n%s %s\n", item.ItemType, item.ItemKey)
			choice, err := promptApplyDecision()
			if err != nil {
				return err
			}
			switch choice {
			case "approve":
			case "reject":
				_ = appendMigrateLog(dir, migrateLogEntry{
					Action:   "rollback",
					Status:   "rejected",
					ItemType: item.ItemType,
					ItemKey:  item.ItemKey,
					URL:      item.URL,
				})
				skipped++
				continue
			case "skip":
				_ = appendMigrateLog(dir, migrateLogEntry{
					Action:   "rollback",
					Status:   "skipped",
					ItemType: item.ItemType,
					ItemKey:  item.ItemKey,
					URL:      item.URL,
				})
				skipped++
				continue
			case "quit":
				if anyChanges {
					if err := finalizeApplyState(dir, items); err != nil {
						return err
					}
				}
				fmt.Println("Stopped by user.")
				return nil
			}
		}

		updatedAt, err := applyItem(cmd.Context(), client, item, content)
		if err != nil {
			errMsg := err.Error()
			_ = appendMigrateLog(dir, migrateLogEntry{
				Action:   "rollback",
				Status:   "error",
				ItemType: item.ItemType,
				ItemKey:  item.ItemKey,
				URL:      item.URL,
				Message:  errMsg,
			})
			skipped++
			continue
		}

		item.InputHash = hashHex(content)
		item.OutputHash = ""
		item.Applied = false
		item.AppliedAt = ""
		item.ApplyError = ""
		item.RollbackAt = time.Now().Format(time.RFC3339)
		item.RollbackError = ""
		item.FetchedAt = time.Now().Format(time.RFC3339)
		if updatedAt != "" {
			item.UpdatedAt = updatedAt
		}

		if err := writeItems(dir, items); err != nil {
			return err
		}
		if err := gitAdd(dir, path, "items.jsonl"); err != nil {
			return err
		}
		if gitHasChanges(dir) {
			if err := gitCommit(dir, fmt.Sprintf("rollback: %s %s", item.ItemType, item.ItemKey)); err != nil {
				return err
			}
			anyChanges = true
		}

		_ = appendMigrateLog(dir, migrateLogEntry{
			Action:   "rollback",
			Status:   "rolled_back",
			ItemType: item.ItemType,
			ItemKey:  item.ItemKey,
			URL:      item.URL,
		})
		rolledBack++
	}

	if anyChanges {
		if err := finalizeApplyState(dir, items); err != nil {
			return err
		}
	}

	fmt.Printf("Rolled back: %d, Skipped: %d\n", rolledBack, skipped)
	return nil
}

func runMigrateList(cmd *cobra.Command, args []string) error {
	dir, err := migrationDir()
	if err != nil {
		return err
	}
	if _, err := loadMetadata(dir); err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}

	items, err := readItems(dir)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		fmt.Println("No items found.")
		return nil
	}

	for i := range items {
		item := &items[i]
		if item.ItemType == "comment" {
			continue
		}
		if !listDiff && !item.Changed {
			continue
		}
		path, err := resolveItemPath(dir, item)
		if err != nil {
			return err
		}
		status := "pending"
		if item.Applied {
			status = "applied"
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", status, item.ItemType, item.ItemKey, item.URL)
		if listDiff {
			if err := printGitDiff(dir, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func runMigrateLogs(cmd *cobra.Command, args []string) error {
	dir, err := migrationDir()
	if err != nil {
		return err
	}
	if _, err := loadMetadata(dir); err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}

	path := migrateLogPath(dir)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("No logs found.")
			return nil
		}
		return fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	entries := make([]migrateLogEntry, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry migrateLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return fmt.Errorf("decode log: %w", err)
		}
		if entry.Status == "no_change" && !migrateLogsAll {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan log: %w", err)
	}

	if migrateLogsLimit > 0 && len(entries) > migrateLogsLimit {
		entries = entries[len(entries)-migrateLogsLimit:]
	}

	for _, entry := range entries {
		ts := entry.TS.Format(time.RFC3339)
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", ts, entry.Action, entry.Status, entry.ItemType, entry.ItemKey)
		if entry.URL != "" {
			fmt.Printf("  %s\n", entry.URL)
		}
		if entry.Message != "" {
			fmt.Printf("  %s\n", entry.Message)
		}
	}
	return nil
}

func runMigrateStatus(cmd *cobra.Command, args []string) error {
	dir, err := migrationDir()
	if err != nil {
		return err
	}

	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("Workspace not found: %s\n", dir)
			return nil
		}
		return fmt.Errorf("stat workspace: %w", err)
	}

	meta, err := loadMetadata(dir)
	if err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}

	items, err := readItems(dir)
	if err != nil {
		return err
	}

	total := 0
	applied := 0
	changed := 0
	byType := map[string]int{}
	for _, item := range items {
		if item.ItemType == "comment" {
			continue
		}
		total++
		byType[normalizedItemType(item.ItemType)]++
		if item.Applied {
			applied++
		}
		if item.Changed {
			changed++
		}
	}

	fmt.Printf("Workspace: %s\n", dir)
	if meta.ProjectKey != "" {
		fmt.Printf("Project: %s (%s)\n", meta.ProjectKey, meta.ProjectName)
	}
	if meta.UpdatedAt != "" {
		fmt.Printf("Updated: %s\n", meta.UpdatedAt)
	}
	fmt.Printf("Total: %d\n", total)
	if len(byType) > 0 {
		types := make([]string, 0, len(byType))
		for key := range byType {
			types = append(types, key)
		}
		sort.Strings(types)
		parts := make([]string, 0, len(types))
		for _, key := range types {
			parts = append(parts, fmt.Sprintf("%s=%d", key, byType[key]))
		}
		fmt.Printf("By type: %s\n", strings.Join(parts, ", "))
	}
	fmt.Printf("Applied: %d\n", applied)
	fmt.Printf("Pending: %d\n", total-applied)
	fmt.Printf("Changed: %d\n", changed)
	return nil
}

func runMigrateClean(cmd *cobra.Command, args []string) error {
	dir, err := migrationDir()
	if err != nil {
		return err
	}

	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("Workspace not found: %s\n", dir)
			return nil
		}
		return fmt.Errorf("stat workspace: %w", err)
	}
	if _, err := loadMetadata(dir); err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}

	if !migrateCleanForce {
		confirm := false
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Remove workspace %s?", dir),
			Default: false,
		}
		if err := survey.AskOne(prompt, &confirm); err != nil {
			return err
		}
		if !confirm {
			fmt.Println("Canceled.")
			return nil
		}
	}

	if err := removeWorkspaceContents(dir); err != nil {
		return err
	}
	_ = appendMigrateLog(dir, migrateLogEntry{
		Action: "clean",
		Status: "cleanup",
	})
	fmt.Printf("Cleaned workspace: %s\n", dir)
	return nil
}

func runMigrateSnapshot(cmd *cobra.Command, args []string) error {
	if !snapshotAppend {
		return fmt.Errorf("only --append is supported for snapshot")
	}
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	dir, err := migrationDir()
	if err != nil {
		return err
	}
	meta, err := loadMetadata(dir)
	if err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}
	if meta.ProjectKey == "" {
		return fmt.Errorf("metadata missing project key")
	}

	if _, err := ensureMigrationRepo(dir, true); err != nil {
		return err
	}

	items, err := readItems(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("items.jsonl not found; run init first")
		}
		return err
	}

	existing := buildItemIndexByIdentity(items)
	project, err := client.GetProject(cmd.Context(), meta.ProjectKey)
	if err != nil {
		return err
	}
	baseURL := fmt.Sprintf("https://%s.%s", cfg.CurrentProfile().Space, cfg.CurrentProfile().Domain)

	newItems := make([]migrateItem, 0)
	issues, err := fetchAllIssues(cmd.Context(), client, project.ID)
	if err != nil {
		return err
	}
	for _, issue := range issues {
		if !issue.IssueKey.IsSet() || issue.IssueKey.Value == "" {
			continue
		}
		issueKey := issue.IssueKey.Value
		if existing[identityKey("issue", issueKey, 0)] {
			continue
		}
		detail, err := client.GetIssue(cmd.Context(), issueKey)
		if err != nil {
			return fmt.Errorf("failed to get issue %s: %w", issueKey, err)
		}
		if !detail.ID.IsSet() {
			return fmt.Errorf("issue %s has no id", issueKey)
		}
		content := optStringValue(detail.Description)
		path := itemContentPath(dir, "issue", issueKey, detail.ID.Value)
		if err := writeItemContent(path, content); err != nil {
			return err
		}
		url := fmt.Sprintf("%s/view/%s", baseURL, issueKey)
		item := migrateItem{
			ItemType:   "issue",
			ItemID:     detail.ID.Value,
			ItemKey:    issueKey,
			URL:        url,
			ProjectKey: meta.ProjectKey,
			Path:       path,
			FetchedAt:  time.Now().Format(time.RFC3339),
			UpdatedAt:  optStringValue(detail.Updated),
			InputHash:  hashHex(content),
		}
		items = append(items, item)
		newItems = append(newItems, item)
		existing[identityKey(item.ItemType, item.ItemKey, item.ItemID)] = true
	}

	wikis, err := client.GetWikis(cmd.Context(), meta.ProjectKey)
	if err != nil {
		return fmt.Errorf("failed to get wikis: %w", err)
	}
	for _, wiki := range wikis {
		key := identityKey("wiki", "", wiki.ID)
		if existing[key] {
			continue
		}
		full, err := client.GetWiki(cmd.Context(), wiki.ID)
		if err != nil {
			return fmt.Errorf("failed to get wiki %d: %w", wiki.ID, err)
		}
		content := full.Content
		path := itemContentPath(dir, "wiki", full.Name, full.ID)
		if err := writeItemContent(path, content); err != nil {
			return err
		}
		url := fmt.Sprintf("%s/alias/wiki/%d", baseURL, wiki.ID)
		if err := writeWikiMetadata(dir, full.ID, full.Name, url, full.Updated); err != nil {
			return err
		}
		item := migrateItem{
			ItemType:   "wiki",
			ItemID:     full.ID,
			ItemKey:    full.Name,
			URL:        url,
			ProjectKey: meta.ProjectKey,
			Path:       path,
			FetchedAt:  time.Now().Format(time.RFC3339),
			UpdatedAt:  full.Updated,
			InputHash:  hashHex(content),
		}
		items = append(items, item)
		newItems = append(newItems, item)
		existing[identityKey(item.ItemType, item.ItemKey, item.ItemID)] = true
	}

	issueTypes, err := client.GetIssueTypes(cmd.Context(), meta.ProjectKey)
	if err != nil {
		return fmt.Errorf("failed to get issue types: %w", err)
	}
	for _, issueType := range issueTypes {
		key := identityKey("issue_type_description", "", issueType.ID)
		if existing[key] {
			continue
		}
		content := issueType.TemplateDescription
		path := itemContentPath(dir, "issue_type_description", issueType.Name, issueType.ID)
		if err := writeItemContent(path, content); err != nil {
			return err
		}
		url := fmt.Sprintf("%s/EditIssueType.action?projectId=%d", baseURL, project.ID)
		if err := writeIssueTypeMetadata(dir, issueType.ID, issueType.Name, url, ""); err != nil {
			return err
		}
		item := migrateItem{
			ItemType:   "issue_type_description",
			ItemID:     issueType.ID,
			ItemKey:    issueType.Name,
			URL:        url,
			ProjectKey: meta.ProjectKey,
			Path:       path,
			FetchedAt:  time.Now().Format(time.RFC3339),
			UpdatedAt:  "",
			InputHash:  hashHex(content),
		}
		items = append(items, item)
		newItems = append(newItems, item)
		existing[identityKey(item.ItemType, item.ItemKey, item.ItemID)] = true
	}

	if len(newItems) == 0 {
		fmt.Println("No new items to append.")
		return nil
	}

	if err := writeItems(dir, items); err != nil {
		return err
	}
	if err := touchMetadata(dir); err != nil {
		return err
	}
	if err := gitAdd(dir, "items.jsonl", "metadata.json", "issue", "wiki", "issue-type"); err != nil {
		return err
	}
	if gitHasChanges(dir) {
		if err := gitCommit(dir, fmt.Sprintf("snapshot: append (%d)", len(newItems))); err != nil {
			return err
		}
	}

	fmt.Printf("Appended %d items.\n", len(newItems))
	return nil
}

type processTarget struct {
	Type        string
	ID          int
	ParentID    int
	Key         string
	URL         string
	ProjectKey  string
	Content     string
	Updated     string
	Attachments []string
	Force       bool
}

type migrateMetadata struct {
	ProjectKey  string `json:"project_key"`
	ProjectName string `json:"project_name"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	BaseBranch  string `json:"base_branch"`
}

type wikiMetadata struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type issueTypeMetadata struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type migrateLogEntry struct {
	TS       time.Time `json:"ts"`
	Action   string    `json:"action"`
	Status   string    `json:"status"`
	ItemType string    `json:"item_type,omitempty"`
	ItemKey  string    `json:"item_key,omitempty"`
	URL      string    `json:"url,omitempty"`
	Message  string    `json:"message,omitempty"`
}

type migrateItem struct {
	ItemType       string                         `json:"item_type"`
	ItemID         int                            `json:"item_id"`
	ParentID       int                            `json:"parent_id,omitempty"`
	ItemKey        string                         `json:"item_key"`
	URL            string                         `json:"url"`
	ProjectKey     string                         `json:"project_key"`
	Path           string                         `json:"path"`
	FetchedAt      string                         `json:"fetched_at"`
	UpdatedAt      string                         `json:"updated_at"`
	DetectedMode   markdown.Mode                  `json:"detected_mode"`
	Score          int                            `json:"score"`
	Rules          []markdown.RuleID              `json:"rules,omitempty"`
	Warnings       map[markdown.WarningType]int   `json:"warnings,omitempty"`
	WarningLines   map[markdown.WarningType][]int `json:"warning_lines,omitempty"`
	Changed        bool                           `json:"changed"`
	InputHash      string                         `json:"input_hash"`
	OutputHash     string                         `json:"output_hash"`
	ConvertForce   bool                           `json:"convert_force"`
	Applied        bool                           `json:"applied"`
	AppliedAt      string                         `json:"applied_at,omitempty"`
	ApplyError     string                         `json:"apply_error,omitempty"`
	SnapshotCommit string                         `json:"snapshot_commit,omitempty"`
	ApplyCommit    string                         `json:"apply_commit,omitempty"`
	RollbackAt     string                         `json:"rollback_at,omitempty"`
	RollbackError  string                         `json:"rollback_error,omitempty"`
}

type lockInfo struct {
	PID  int    `json:"pid"`
	Time string `json:"time"`
	Cmd  string `json:"cmd"`
}

type currentItem struct {
	Content     string
	Updated     string
	Attachments []string
	Name        string
}

func migrationDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd: %w", err)
	}
	if migrateWorkspaceDir == "" {
		return cwd, nil
	}
	if filepath.IsAbs(migrateWorkspaceDir) {
		return migrateWorkspaceDir, nil
	}
	return filepath.Join(cwd, migrateWorkspaceDir), nil
}

func ensureMigrationRepo(dir string, ensureGitignoreFlag bool) (string, error) {
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat git dir: %w", err)
		}
		if err := gitInit(dir); err != nil {
			return "", err
		}
	}
	if ensureGitignoreFlag {
		if err := ensureGitignore(dir); err != nil {
			return "", err
		}
	}
	branch, err := gitCurrentBranch(dir)
	if err != nil || branch == "" || branch == "HEAD" {
		branch = "master"
	}
	return branch, nil
}

func ensureGitignore(dir string) error {
	path := filepath.Join(dir, ".gitignore")
	entry := "logs.jsonl"
	data, err := os.ReadFile(path)
	if err == nil {
		if strings.Contains(string(data), entry) {
			return nil
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read gitignore: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("write gitignore: %w", err)
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, entry); err != nil {
		return fmt.Errorf("append gitignore: %w", err)
	}
	return nil
}

func loadMetadata(dir string) (migrateMetadata, error) {
	path := filepath.Join(dir, "metadata.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return migrateMetadata{}, err
	}
	var meta migrateMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return migrateMetadata{}, err
	}
	return meta, nil
}

func itemContentPath(dir, itemType, itemKey string, itemID int) string {
	if itemType == "wiki" && itemID > 0 {
		return filepath.Join(dir, itemType, fmt.Sprintf("%d", itemID), "content.md")
	}
	if itemType == "issue_type_description" && itemID > 0 {
		return filepath.Join(dir, "issue-type", fmt.Sprintf("%d", itemID), "description.md")
	}
	return filepath.Join(dir, itemType, safePath(itemKey), "content.md")
}

func resolveItemPath(dir string, item *migrateItem) (string, error) {
	if item.ItemType == "wiki" {
		desired := itemContentPath(dir, "wiki", item.ItemKey, item.ItemID)
		if item.Path == "" || item.Path == desired {
			item.Path = desired
			return desired, nil
		}
		item.Path = desired
		return desired, nil
	}
	if item.Path == "" {
		item.Path = itemContentPath(dir, item.ItemType, item.ItemKey, item.ItemID)
	}
	return item.Path, nil
}

func writeWikiMetadata(dir string, wikiID int, name, url, updatedAt string) error {
	if wikiID == 0 {
		return nil
	}
	meta := wikiMetadata{
		ID:        wikiID,
		Name:      name,
		URL:       url,
		UpdatedAt: updatedAt,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal wiki metadata: %w", err)
	}
	path := filepath.Join(dir, "wiki", fmt.Sprintf("%d", wikiID), "metadata.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create wiki dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write wiki metadata: %w", err)
	}
	return nil
}

func writeIssueTypeMetadata(dir string, issueTypeID int, name, url, updatedAt string) error {
	if issueTypeID == 0 {
		return nil
	}
	meta := issueTypeMetadata{
		ID:        issueTypeID,
		Name:      name,
		URL:       url,
		UpdatedAt: updatedAt,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal issue type metadata: %w", err)
	}
	path := filepath.Join(dir, "issue-type", fmt.Sprintf("%d", issueTypeID), "metadata.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create issue type dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write issue type metadata: %w", err)
	}
	return nil
}

func readFileIfExists(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func writeItemContent(path string, content string) error {
	if path == "" {
		return errors.New("content path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create content dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write content: %w", err)
	}
	return nil
}

func finalizeApplyState(dir string, items []migrateItem) error {
	if err := writeItems(dir, items); err != nil {
		return err
	}
	if err := touchMetadata(dir); err != nil {
		return err
	}
	if err := gitAdd(dir, "items.jsonl", "metadata.json"); err != nil {
		return err
	}
	if gitHasChanges(dir) {
		if err := gitCommit(dir, fmt.Sprintf("state: %s", time.Now().Format("20060102-150405"))); err != nil {
			return err
		}
	}
	return nil
}

func snapshotAll(ctx context.Context, client *api.Client, projectKey string, projectID int, dir string, baseURL string) ([]migrateItem, error) {
	items := make([]migrateItem, 0)
	issues, err := fetchAllIssues(ctx, client, projectID)
	if err != nil {
		return nil, err
	}
	for _, issue := range issues {
		if !issue.IssueKey.IsSet() || issue.IssueKey.Value == "" {
			continue
		}
		issueKey := issue.IssueKey.Value
		detail, err := client.GetIssue(ctx, issueKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get issue %s: %w", issueKey, err)
		}
		if !detail.ID.IsSet() {
			return nil, fmt.Errorf("issue %s has no id", issueKey)
		}
		url := fmt.Sprintf("%s/view/%s", baseURL, issueKey)
		content := optStringValue(detail.Description)
		path := itemContentPath(dir, "issue", issueKey, detail.ID.Value)
		if err := writeItemContent(path, content); err != nil {
			return nil, err
		}
		items = append(items, migrateItem{
			ItemType:   "issue",
			ItemID:     detail.ID.Value,
			ItemKey:    issueKey,
			URL:        url,
			ProjectKey: projectKey,
			Path:       path,
			FetchedAt:  time.Now().Format(time.RFC3339),
			UpdatedAt:  optStringValue(detail.Updated),
			InputHash:  hashHex(content),
		})
	}
	wikis, err := client.GetWikis(ctx, projectKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get wikis: %w", err)
	}
	for _, wiki := range wikis {
		full, err := client.GetWiki(ctx, wiki.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get wiki %d: %w", wiki.ID, err)
		}
		url := fmt.Sprintf("%s/alias/wiki/%d", baseURL, wiki.ID)
		content := full.Content
		path := itemContentPath(dir, "wiki", full.Name, full.ID)
		if err := writeItemContent(path, content); err != nil {
			return nil, err
		}
		if err := writeWikiMetadata(dir, full.ID, full.Name, url, full.Updated); err != nil {
			return nil, err
		}
		items = append(items, migrateItem{
			ItemType:   "wiki",
			ItemID:     full.ID,
			ItemKey:    full.Name,
			URL:        url,
			ProjectKey: projectKey,
			Path:       path,
			FetchedAt:  time.Now().Format(time.RFC3339),
			UpdatedAt:  full.Updated,
			InputHash:  hashHex(content),
		})
	}
	issueTypes, err := client.GetIssueTypes(ctx, projectKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get issue types: %w", err)
	}
	for _, issueType := range issueTypes {
		url := fmt.Sprintf("%s/EditIssueType.action?projectId=%d", baseURL, projectID)
		description := issueType.TemplateDescription
		descriptionPath := itemContentPath(dir, "issue_type_description", issueType.Name, issueType.ID)
		if err := writeItemContent(descriptionPath, description); err != nil {
			return nil, err
		}
		if err := writeIssueTypeMetadata(dir, issueType.ID, issueType.Name, url, ""); err != nil {
			return nil, err
		}
		items = append(items, migrateItem{
			ItemType:   "issue_type_description",
			ItemID:     issueType.ID,
			ItemKey:    issueType.Name,
			URL:        url,
			ProjectKey: projectKey,
			Path:       descriptionPath,
			FetchedAt:  time.Now().Format(time.RFC3339),
			UpdatedAt:  "",
			InputHash:  hashHex(description),
		})
	}
	return items, nil
}

func migrateLogPath(dir string) string {
	return filepath.Join(dir, "logs.jsonl")
}

func appendMigrateLog(dir string, entry migrateLogEntry) error {
	if dir == "" {
		return nil
	}
	entry.TS = time.Now()
	path := migrateLogPath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	if err := enc.Encode(entry); err != nil {
		return fmt.Errorf("write log entry: %w", err)
	}
	return nil
}

func gitInit(dir string) error {
	_, err := runGit(dir, "init")
	return err
}

func gitCurrentBranch(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func gitHasChanges(dir string) bool {
	out, err := runGit(dir, "status", "--porcelain")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

func gitAdd(dir string, paths ...string) error {
	args := append([]string{"add", "--"}, paths...)
	_, err := runGit(dir, args...)
	return err
}

func gitCommit(dir, message string) error {
	_, err := runGit(dir, "commit", "-m", message)
	return err
}

func gitCommitAllowEmpty(dir, message string) error {
	_, err := runGit(dir, "commit", "--allow-empty", "-m", message)
	return err
}

func gitCheckout(dir, branch string) error {
	_, err := runGit(dir, "checkout", branch)
	return err
}

func gitCheckoutNewBranch(dir, branch string) error {
	_, err := runGit(dir, "checkout", "-b", branch)
	return err
}

func gitMergeSquash(dir, branch string) error {
	_, err := runGit(dir, "merge", "--squash", branch)
	return err
}

func gitMergeNoFF(dir, branch string) error {
	_, err := runGit(dir, "merge", "--no-ff", branch)
	return err
}

func gitResetSoftTo(dir, ref string) error {
	_, err := runGit(dir, "reset", "--soft", ref)
	return err
}

func gitCheckoutNewBranchFrom(dir, branch, startPoint string) error {
	_, err := runGit(dir, "checkout", "-b", branch, startPoint)
	return err
}

func gitDeleteBranch(dir, branch string) error {
	_, err := runGit(dir, "branch", "-D", branch)
	return err
}

func gitBranchExists(dir, branch string) bool {
	_, err := runGit(dir, "show-ref", "--verify", fmt.Sprintf("refs/heads/%s", branch))
	return err == nil
}

func gitHasCommits(dir string) bool {
	_, err := runGit(dir, "rev-parse", "--verify", "HEAD")
	return err == nil
}

func resolveBaseBranch(dir, preferred string) (string, error) {
	if preferred != "" && gitBranchExists(dir, preferred) {
		return preferred, nil
	}
	if gitBranchExists(dir, "main") {
		return "main", nil
	}
	if gitBranchExists(dir, "master") {
		return "master", nil
	}
	branch, err := gitCurrentBranch(dir)
	if err != nil {
		return "", err
	}
	return branch, nil
}

func checkoutIfPossible(dir, branch string) bool {
	if branch == "" || branch == "HEAD" {
		return false
	}
	if !gitBranchExists(dir, branch) {
		return false
	}
	if err := gitCheckout(dir, branch); err != nil {
		fmt.Printf("Failed to checkout %s: %v\n", branch, err)
		return false
	}
	return true
}

func removeWorkspaceContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read workspace: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "metadata.json" {
			continue
		}
		path := filepath.Join(dir, name)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove %s: %w", name, err)
		}
	}
	return nil
}

func isDirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("read dir: %w", err)
	}
	return len(entries) == 0, nil
}

func matchesRollbackTarget(item *migrateItem, targets map[string]bool) bool {
	if targets[item.ItemKey] {
		return true
	}
	if item.ItemType == "wiki" {
		return targets[fmt.Sprintf("%d", item.ItemID)]
	}
	if item.ItemType == "issue_type_description" {
		return targets[fmt.Sprintf("%d", item.ItemID)]
	}
	return false
}

func buildUnsafeRuleSet(values []string) map[markdown.RuleID]bool {
	if len(values) == 0 {
		return nil
	}
	allowed := make(map[markdown.RuleID]bool, len(values))
	for _, rule := range values {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		allowed[markdown.RuleID(rule)] = true
	}
	return allowed
}

func gitFileCommits(dir, path string, limit int) ([]string, error) {
	args := []string{"log", "--pretty=format:%H", "--"}
	if limit > 0 {
		args = append([]string{"log", "--pretty=format:%H", fmt.Sprintf("-%d", limit), "--"}, path)
	} else {
		args = append(args, path)
	}
	out, err := runGit(dir, args...)
	if err != nil {
		return nil, err
	}
	lines := strings.Fields(out)
	return lines, nil
}

func gitDiff(dir, from, to, path string) error {
	cmd := exec.Command("git", "diff", from, to, "--", path)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("git diff failed: %w", err)
}

func gitFirstCommitForFile(dir, path string) (string, error) {
	out, err := runGit(dir, "log", "--reverse", "--pretty=format:%H", "--", path)
	if err != nil {
		return "", err
	}
	lines := strings.Fields(out)
	if len(lines) == 0 {
		return "", nil
	}
	return lines[0], nil
}

func gitCheckoutFile(dir, commit, path string) error {
	_, err := runGit(dir, "checkout", commit, "--", path)
	return err
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func ensureMetadata(dir, projectKey, projectName, baseBranch string) error {
	path := filepath.Join(dir, "metadata.json")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat metadata: %w", err)
	}

	meta := migrateMetadata{
		ProjectKey:  projectKey,
		ProjectName: projectName,
		CreatedAt:   time.Now().Format(time.RFC3339),
		UpdatedAt:   time.Now().Format(time.RFC3339),
		BaseBranch:  baseBranch,
	}
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}

func touchMetadata(dir string) error {
	path := filepath.Join(dir, "metadata.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read metadata: %w", err)
	}
	var meta migrateMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("unmarshal metadata: %w", err)
	}
	meta.UpdatedAt = time.Now().Format(time.RFC3339)
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}

func acquireLock(dir string, force bool) (func() error, error) {
	path := filepath.Join(dir, "lock")
	if force {
		_ = os.Remove(path)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("lock already exists: %s", path)
		}
		return nil, fmt.Errorf("create lock: %w", err)
	}

	info := lockInfo{PID: os.Getpid(), Time: time.Now().Format(time.RFC3339), Cmd: strings.Join(os.Args, " ")}
	data, _ := json.Marshal(info)
	_, _ = file.Write(append(data, '\n'))
	_ = file.Close()

	return func() error { return os.Remove(path) }, nil
}

func writeItems(dir string, items []migrateItem) error {
	path := filepath.Join(dir, "items.jsonl")
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open items file: %w", err)
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	for _, item := range items {
		if err := enc.Encode(item); err != nil {
			return fmt.Errorf("write items: %w", err)
		}
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close items file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace items file: %w", err)
	}
	return nil
}

func readItems(dir string) ([]migrateItem, error) {
	path := filepath.Join(dir, "items.jsonl")
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open items file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	items := make([]migrateItem, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item migrateItem
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("decode item: %w", err)
		}
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan items: %w", err)
	}
	return items, nil
}

func readItemsIfExists(dir string) ([]migrateItem, error) {
	path := filepath.Join(dir, "items.jsonl")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat items file: %w", err)
	}
	return readItems(dir)
}

func fetchAllIssues(ctx context.Context, client *api.Client, projectID int) ([]backlog.Issue, error) {
	all := make([]backlog.Issue, 0)
	offset := 0
	for {
		issues, err := client.GetIssues(ctx, &api.IssueListOptions{
			ProjectIDs: []int{projectID},
			Offset:     offset,
			Count:      100,
			Order:      "asc",
		})
		if err != nil {
			return nil, err
		}
		if len(issues) == 0 {
			break
		}
		all = append(all, issues...)
		if len(issues) < 100 {
			break
		}
		offset += 100
	}
	return all, nil
}

func fetchAllComments(ctx context.Context, client *api.Client, issueKey string) ([]api.Comment, error) {
	all := make([]api.Comment, 0)
	minID := 0
	for {
		comments, err := client.GetComments(ctx, issueKey, &api.CommentListOptions{
			MinID: minID,
			Count: 100,
			Order: "asc",
		})
		if err != nil {
			return nil, err
		}
		if len(comments) == 0 {
			break
		}
		all = append(all, comments...)
		minID = comments[len(comments)-1].ID + 1
		if len(comments) < 100 {
			break
		}
	}
	return all, nil
}

func fetchCurrentItem(ctx context.Context, client *api.Client, item *migrateItem) (*currentItem, error) {
	switch item.ItemType {
	case "issue":
		issue, err := client.GetIssue(ctx, item.ItemKey)
		if err != nil {
			return nil, err
		}
		return &currentItem{
			Content:     optStringValue(issue.Description),
			Updated:     optStringValue(issue.Updated),
			Attachments: attachmentNamesFromBacklog(issue.Attachments),
		}, nil
	case "comment":
		parts := strings.Split(item.ItemKey, "#comment-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid comment key: %s", item.ItemKey)
		}
		commentID, err := parseInt(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid comment id: %w", err)
		}
		comment, err := client.GetComment(ctx, parts[0], commentID)
		if err != nil {
			return nil, err
		}
		return &currentItem{
			Content:     comment.Content,
			Updated:     comment.Updated,
			Attachments: nil,
		}, nil
	case "wiki":
		wiki, err := client.GetWiki(ctx, item.ItemID)
		if err != nil {
			return nil, err
		}
		return &currentItem{
			Content:     wiki.Content,
			Updated:     wiki.Updated,
			Attachments: attachmentNamesFromAPI(wiki.Attachments),
			Name:        wiki.Name,
		}, nil
	case "issue_type_description":
		issueType, err := getIssueType(ctx, client, item.ProjectKey, item.ItemID)
		if err != nil {
			return nil, err
		}
		return &currentItem{
			Content:     issueType.TemplateDescription,
			Updated:     "",
			Attachments: nil,
		}, nil
	default:
		return nil, fmt.Errorf("unknown item type: %s", item.ItemType)
	}
}

var issueTypeCache = map[string]map[int]api.IssueType{}

func getIssueType(ctx context.Context, client *api.Client, projectKey string, issueTypeID int) (api.IssueType, error) {
	if projectKey == "" {
		return api.IssueType{}, fmt.Errorf("project key is required for issue type lookup")
	}
	if cache, ok := issueTypeCache[projectKey]; ok {
		if issueType, ok := cache[issueTypeID]; ok {
			return issueType, nil
		}
	}
	issueTypes, err := client.GetIssueTypes(ctx, projectKey)
	if err != nil {
		return api.IssueType{}, err
	}
	cache := make(map[int]api.IssueType, len(issueTypes))
	for _, issueType := range issueTypes {
		cache[issueType.ID] = issueType
	}
	issueTypeCache[projectKey] = cache
	issueType, ok := cache[issueTypeID]
	if !ok {
		return api.IssueType{}, fmt.Errorf("issue type not found: %d", issueTypeID)
	}
	return issueType, nil
}

func applyItem(ctx context.Context, client *api.Client, item *migrateItem, content string) (string, error) {
	switch item.ItemType {
	case "issue":
		issue, err := client.UpdateIssue(ctx, item.ItemKey, &api.UpdateIssueInput{Description: &content})
		if err != nil {
			return "", err
		}
		return optStringValue(issue.Updated), nil
	case "comment":
		parts := strings.Split(item.ItemKey, "#comment-")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid comment key: %s", item.ItemKey)
		}
		commentID, err := parseInt(parts[1])
		if err != nil {
			return "", fmt.Errorf("invalid comment id: %w", err)
		}
		_, err = client.UpdateComment(ctx, parts[0], commentID, content)
		if err != nil {
			return "", err
		}
		return "", nil
	case "wiki":
		wiki, err := client.UpdateWiki(ctx, item.ItemID, &api.UpdateWikiInput{Content: &content})
		if err != nil {
			return "", err
		}
		return wiki.Updated, nil
	case "issue_type_description":
		_, err := client.UpdateIssueType(ctx, item.ProjectKey, item.ItemID, &api.UpdateIssueTypeInput{TemplateDescription: &content})
		return "", err
	default:
		return "", fmt.Errorf("unknown item type: %s", item.ItemType)
	}
}

func isSourceMatch(item *migrateItem, content, updated string) bool {
	if item.UpdatedAt != "" && updated != "" && item.UpdatedAt != updated {
		return false
	}
	return hashHex(content) == item.InputHash
}

func parseInt(value string) (int, error) {
	var out int
	_, err := fmt.Sscanf(value, "%d", &out)
	return out, err
}

func hashHex(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func optStringValue(value backlog.OptString) string {
	if value.IsSet() {
		return value.Value
	}
	return ""
}

func attachmentNamesFromBacklog(attachments []backlog.Attachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	names := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Name.IsSet() && attachment.Name.Value != "" {
			names = append(names, attachment.Name.Value)
		}
	}
	return names
}

func attachmentNamesFromAPI(attachments []api.Attachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	names := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Name != "" {
			names = append(names, attachment.Name)
		}
	}
	return names
}

func printChangeSummary(item migrateItem) {
	warnings := formatWarnings(item.Warnings)
	lines := formatWarningLines(item.WarningLines)
	rules := formatRules(item.Rules)
	fmt.Printf("%s %s changed rules=%s warnings=%s lines=%s\n", item.ItemType, item.ItemKey, rules, warnings, lines)
}

func printContentDiff(before, after string) error {
	if before == after {
		return nil
	}
	beforeFile, err := os.CreateTemp("", "backlog-md-before-*.md")
	if err != nil {
		return err
	}
	defer os.Remove(beforeFile.Name())
	if _, err := beforeFile.WriteString(before); err != nil {
		beforeFile.Close()
		return err
	}
	_ = beforeFile.Close()

	afterFile, err := os.CreateTemp("", "backlog-md-after-*.md")
	if err != nil {
		return err
	}
	defer os.Remove(afterFile.Name())
	if _, err := afterFile.WriteString(after); err != nil {
		afterFile.Close()
		return err
	}
	_ = afterFile.Close()

	return printDiffFiles(beforeFile.Name(), afterFile.Name())
}

func printGitDiff(dir, path string) error {
	commits, err := gitFileCommits(dir, path, 2)
	if err != nil {
		return err
	}
	if len(commits) < 2 {
		return nil
	}
	return gitDiff(dir, commits[1], commits[0], path)
}

func printDiffFiles(beforePath, afterPath string) error {
	if _, err := exec.LookPath("diff"); err != nil {
		before, err := os.ReadFile(beforePath)
		if err != nil {
			return fmt.Errorf("read before diff: %w", err)
		}
		after, err := os.ReadFile(afterPath)
		if err != nil {
			return fmt.Errorf("read after diff: %w", err)
		}
		fmt.Printf("--- %s\n+++ %s\n", beforePath, afterPath)
		fmt.Println("<<<<< BEFORE")
		fmt.Println(string(before))
		fmt.Println(">>>>> AFTER")
		fmt.Println(string(after))
		return nil
	}

	cmd := exec.Command("diff", "-u", beforePath, afterPath)
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		fmt.Print(string(output))
	}
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("diff failed: %w", err)
}

func formatRules(rules []markdown.RuleID) string {
	if len(rules) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(rules))
	for _, rule := range rules {
		if rule == "" {
			continue
		}
		keys = append(keys, string(rule))
	}
	if len(keys) == 0 {
		return "-"
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func applyConversion(item *migrateItem, content string, attachments []string, unsafeRules map[markdown.RuleID]bool) (string, bool, error) {
	force := item.ConvertForce
	result := markdown.Convert(content, markdown.ConvertOptions{
		Force:           force,
		ItemType:        item.ItemType,
		ItemID:          item.ItemID,
		ParentID:        item.ParentID,
		ProjectKey:      item.ProjectKey,
		ItemKey:         item.ItemKey,
		URL:             item.URL,
		AttachmentNames: attachments,
		UnsafeRules:     unsafeRules,
	})

	changed := result.Output != content
	item.DetectedMode = result.Mode
	item.Score = result.Score
	item.Rules = result.Rules
	item.Warnings = result.Warnings
	item.WarningLines = result.WarningLines
	item.OutputHash = hashHex(result.Output)
	item.Changed = changed

	return result.Output, changed, nil
}

func buildAppliedIndex(items []migrateItem) map[string]migrateItem {
	applied := make(map[string]migrateItem)
	for _, item := range items {
		if !item.Applied {
			continue
		}
		applied[buildItemKey(item.ItemType, item.ItemKey)] = item
	}
	return applied
}

func buildItemKey(itemType, itemKey string) string {
	return strings.ToLower(itemType) + ":" + itemKey
}

func buildItemIndex(items []migrateItem) map[string]migrateItem {
	index := make(map[string]migrateItem, len(items))
	for _, item := range items {
		index[buildItemKey(item.ItemType, item.ItemKey)] = item
	}
	return index
}

func buildItemIndexByIdentity(items []migrateItem) map[string]bool {
	index := make(map[string]bool, len(items))
	for _, item := range items {
		index[identityKey(item.ItemType, item.ItemKey, item.ItemID)] = true
	}
	return index
}

func identityKey(itemType, itemKey string, itemID int) string {
	switch strings.ToLower(itemType) {
	case "issue":
		return "issue:" + itemKey
	case "wiki":
		return "wiki:" + fmt.Sprintf("%d", itemID)
	case "issue_type_description":
		return "issue_type_description:" + fmt.Sprintf("%d", itemID)
	default:
		return strings.ToLower(itemType) + ":" + itemKey
	}
}

func inheritApplyState(item migrateItem, existing migrateItem) migrateItem {
	if existing.ItemKey == "" {
		return item
	}
	item.Applied = existing.Applied
	item.AppliedAt = existing.AppliedAt
	item.ApplyError = existing.ApplyError
	item.RollbackAt = existing.RollbackAt
	item.RollbackError = existing.RollbackError
	return item
}

func normalizeTypes(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		allowed[value] = true
	}
	return allowed
}

func normalizeTargets(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	targets := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		targets[value] = true
	}
	return targets
}

func typeAllowed(allowed map[string]bool, itemType string) bool {
	if len(allowed) == 0 {
		return true
	}
	return allowed[normalizedItemType(itemType)]
}

func promptApplyDecision() (string, error) {
	options := []string{"approve", "reject", "skip", "quit"}
	prompt := &survey.Select{
		Message: "Apply this change?",
		Options: options,
		Default: "approve",
	}
	var choice string
	if err := survey.AskOne(prompt, &choice); err != nil {
		return "", err
	}
	return choice, nil
}

func normalizedItemType(itemType string) string {
	normalized := strings.ToLower(itemType)
	if strings.HasPrefix(normalized, "issue_type_") {
		return "issue_type"
	}
	return normalized
}

func slugify(name string) string {
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "project"
	}
	return out
}

func safePath(value string) string {
	if value == "" {
		return "item"
	}
	return slugify(value)
}

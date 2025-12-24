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

const migrationBaseDir = "backlog-markdown-migrate"

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate Backlog notation to GFM",
	Long: `Migrate Backlog notation to GFM for a project.

This command suite supports separate init/convert/apply/rollback steps to
control API usage and resume safely. Workspace data is stored in
./backlog-markdown-migrate/<projectKey> under the current directory.

Examples:
  backlog markdown migrate init
  backlog markdown migrate convert
  backlog markdown migrate apply
  backlog markdown migrate rollback
  backlog markdown migrate list`,
}

var migrateInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a migration workspace",
	RunE:  runMigrateInit,
}

var (
	convertForce     bool
	convertForceLock bool
)

var migrateConvertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Fetch and convert data into the workspace",
	RunE:  runMigrateConvert,
}

var (
	applyForceLock bool
	applyAuto      bool
	applyTypes     []string
)

var migrateApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply converted data back to Backlog",
	RunE:  runMigrateApply,
}

var (
	rollbackForceLock bool
	rollbackTargets   []string
	rollbackTypes     []string
)

var migrateRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback applied conversions",
	RunE:  runMigrateRollback,
}

var migrateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List items in the migration workspace",
	RunE:  runMigrateList,
}

var listDiff bool

func init() {
	migrateConvertCmd.Flags().BoolVar(&convertForce, "force", false, "Convert even when detection is unknown")
	migrateConvertCmd.Flags().BoolVar(&convertForceLock, "force-lock", false, "Remove existing lock and retry")
	migrateApplyCmd.Flags().BoolVar(&applyForceLock, "force-lock", false, "Remove existing lock and retry")
	migrateApplyCmd.Flags().BoolVar(&applyAuto, "auto", false, "Apply changes without confirmation")
	migrateApplyCmd.Flags().StringSliceVar(&applyTypes, "types", nil, "Apply target types (issue,comment,wiki). Default: all")
	migrateRollbackCmd.Flags().BoolVar(&rollbackForceLock, "force-lock", false, "Remove existing lock and retry")
	migrateRollbackCmd.Flags().StringSliceVar(&rollbackTargets, "targets", nil, "Rollback target item keys (issue key, comment key, wiki name)")
	migrateRollbackCmd.Flags().StringSliceVar(&rollbackTypes, "types", nil, "Rollback target types (issue,comment,wiki). Default: all")
	migrateListCmd.Flags().BoolVar(&listDiff, "diff", false, "Show diffs for changed items")

	migrateCmd.AddCommand(migrateInitCmd)
	migrateCmd.AddCommand(migrateConvertCmd)
	migrateCmd.AddCommand(migrateApplyCmd)
	migrateCmd.AddCommand(migrateRollbackCmd)
	migrateCmd.AddCommand(migrateListCmd)
	MarkdownCmd.AddCommand(migrateCmd)
}

func runMigrateInit(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	project, err := client.GetProject(cmd.Context(), projectKey)
	if err != nil {
		return err
	}

	dir, err := migrationDir(projectKey)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create migration dir: %w", err)
	}

	if err := ensureMetadata(dir, project.ProjectKey, project.Name); err != nil {
		return err
	}

	fmt.Printf("Workspace: %s\n", dir)
	return nil
}

func runMigrateConvert(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	ctx := cmd.Context()

	project, err := client.GetProject(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	dir, err := migrationDir(projectKey)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create migration dir: %w", err)
	}
	if err := ensureMetadata(dir, project.ProjectKey, project.Name); err != nil {
		return err
	}

	release, err := acquireLock(dir, convertForceLock)
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	items := make([]migrateItem, 0)
	existingItems, err := readItemsIfExists(dir)
	if err != nil {
		return err
	}
	existingApplied := buildAppliedIndex(existingItems)

	changedCount := 0

	issues, err := fetchAllIssues(ctx, client, project.ID)
	if err != nil {
		return err
	}

	for _, issue := range issues {
		if !issue.IssueKey.IsSet() || issue.IssueKey.Value == "" {
			continue
		}
		issueKey := issue.IssueKey.Value
		detail, err := client.GetIssue(ctx, issueKey)
		if err != nil {
			return fmt.Errorf("failed to get issue %s: %w", issueKey, err)
		}
		if !detail.ID.IsSet() {
			return fmt.Errorf("issue %s has no id", issueKey)
		}

		url := fmt.Sprintf("https://%s.%s/view/%s", cfg.CurrentProfile().Space, cfg.CurrentProfile().Domain, issueKey)
		if appliedItem, ok := existingApplied[buildItemKey("issue", issueKey)]; ok {
			items = append(items, appliedItem)
			continue
		}

		item, changed, err := convertItem(processTarget{
			Type:       "issue",
			ID:         detail.ID.Value,
			Key:        issueKey,
			URL:        url,
			ProjectKey: projectKey,
			Content:    optStringValue(detail.Description),
			Updated:    optStringValue(detail.Updated),
			Force:      convertForce,
		}, dir)
		if err != nil {
			return err
		}
		items = append(items, item)
		if changed {
			changedCount++
			printChangeSummary(item)
		}

		comments, err := fetchAllComments(ctx, client, issueKey)
		if err != nil {
			return fmt.Errorf("failed to get comments for %s: %w", issueKey, err)
		}
		for _, comment := range comments {
			commentURL := fmt.Sprintf("%s#comment-%d", url, comment.ID)
			commentKey := fmt.Sprintf("%s#comment-%d", issueKey, comment.ID)
			if appliedItem, ok := existingApplied[buildItemKey("comment", commentKey)]; ok {
				items = append(items, appliedItem)
				continue
			}

			item, changed, err := convertItem(processTarget{
				Type:       "comment",
				ID:         comment.ID,
				ParentID:   detail.ID.Value,
				Key:        commentKey,
				URL:        commentURL,
				ProjectKey: projectKey,
				Content:    comment.Content,
				Updated:    comment.Updated,
				Force:      convertForce,
			}, dir)
			if err != nil {
				return err
			}
			items = append(items, item)
			if changed {
				changedCount++
				printChangeSummary(item)
			}
		}
	}

	wikis, err := client.GetWikis(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get wikis: %w", err)
	}
	for _, wiki := range wikis {
		full, err := client.GetWiki(ctx, wiki.ID)
		if err != nil {
			return fmt.Errorf("failed to get wiki %d: %w", wiki.ID, err)
		}
		url := fmt.Sprintf("https://%s.%s/alias/wiki/%d", cfg.CurrentProfile().Space, cfg.CurrentProfile().Domain, wiki.ID)
		if appliedItem, ok := existingApplied[buildItemKey("wiki", full.Name)]; ok {
			items = append(items, appliedItem)
			continue
		}

		item, changed, err := convertItem(processTarget{
			Type:       "wiki",
			ID:         full.ID,
			Key:        full.Name,
			URL:        url,
			ProjectKey: projectKey,
			Content:    full.Content,
			Updated:    full.Updated,
			Force:      convertForce,
		}, dir)
		if err != nil {
			return err
		}
		items = append(items, item)
		if changed {
			changedCount++
			printChangeSummary(item)
		}
	}

	if err := writeItems(dir, items); err != nil {
		return err
	}
	if err := touchMetadata(dir); err != nil {
		return err
	}

	if changedCount > 0 {
		fmt.Printf("Changes: %d\n", changedCount)
	}
	return nil
}

func runMigrateApply(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	ctx := cmd.Context()

	dir, err := migrationDir(projectKey)
	if err != nil {
		return err
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

	for i := range items {
		item := &items[i]
		if !item.Changed || item.Applied {
			continue
		}
		if !typeAllowed(allowedTypes, item.ItemType) {
			continue
		}

		current, err := fetchCurrentItem(ctx, client, item)
		if err != nil {
			item.ApplyError = err.Error()
			skipped++
			continue
		}

		if !isSourceMatch(item, current.Content, current.Updated) {
			item.ApplyError = "source changed"
			if !applyAuto {
				fmt.Printf("Skip %s %s: source changed\n", item.ItemType, item.ItemKey)
			}
			skipped++
			continue
		}

		converted, changed, err := applyConversion(item, current.Content)
		if err != nil {
			item.ApplyError = err.Error()
			skipped++
			continue
		}
		if !changed {
			item.ApplyError = "no changes"
			skipped++
			continue
		}

		if !applyAuto {
			if err := printDiff(item); err != nil {
				return err
			}
			fmt.Printf("\n%s %s\n", item.ItemType, item.ItemKey)
			if item.URL != "" {
				fmt.Printf("%s\n", item.URL)
			}
			choice, err := promptApplyDecision()
			if err != nil {
				return err
			}
			switch choice {
			case "approve":
			case "reject":
				item.ApplyError = "rejected"
				skipped++
				continue
			case "skip":
				item.ApplyError = "skipped"
				skipped++
				continue
			case "quit":
				if err := writeItems(dir, items); err != nil {
					return err
				}
				if err := touchMetadata(dir); err != nil {
					return err
				}
				fmt.Println("Stopped by user.")
				return nil
			}
		}

		if err := applyItem(ctx, client, item, converted); err != nil {
			item.ApplyError = err.Error()
			if !applyAuto {
				fmt.Printf("Failed %s %s: %s\n", item.ItemType, item.ItemKey, item.ApplyError)
			}
			skipped++
			continue
		}

		item.Applied = true
		item.AppliedAt = time.Now().Format(time.RFC3339)
		item.ApplyError = ""
		applied++
	}

	if err := writeItems(dir, items); err != nil {
		return err
	}
	if err := touchMetadata(dir); err != nil {
		return err
	}

	fmt.Printf("Applied: %d, Skipped: %d\n", applied, skipped)
	return nil
}

func runMigrateRollback(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	ctx := cmd.Context()

	dir, err := migrationDir(projectKey)
	if err != nil {
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

	for i := range items {
		item := &items[i]
		if !item.Applied {
			continue
		}
		if !typeAllowed(allowedTypes, item.ItemType) {
			continue
		}
		if len(targets) > 0 && !targets[item.ItemKey] {
			continue
		}

		current, err := fetchCurrentItem(ctx, client, item)
		if err != nil {
			item.RollbackError = err.Error()
			skipped++
			continue
		}

		if hashHex(current.Content) != item.OutputHash {
			item.RollbackError = "source changed"
			skipped++
			continue
		}

		original, err := os.ReadFile(item.OriginalPath)
		if err != nil {
			item.RollbackError = fmt.Sprintf("read original: %v", err)
			skipped++
			continue
		}

		if err := applyItem(ctx, client, item, string(original)); err != nil {
			item.RollbackError = err.Error()
			skipped++
			continue
		}

		item.Applied = false
		item.AppliedAt = ""
		item.RollbackAt = time.Now().Format(time.RFC3339)
		item.RollbackError = ""
		rolledBack++
	}

	if err := writeItems(dir, items); err != nil {
		return err
	}
	if err := touchMetadata(dir); err != nil {
		return err
	}

	fmt.Printf("Rolled back: %d, Skipped: %d\n", rolledBack, skipped)
	return nil
}

func runMigrateList(cmd *cobra.Command, args []string) error {
	_, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	dir, err := migrationDir(projectKey)
	if err != nil {
		return err
	}

	items, err := readItems(dir)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		fmt.Println("No items found.")
		return nil
	}

	for _, item := range items {
		if !item.Changed {
			continue
		}
		status := "pending"
		if item.Applied {
			status = "applied"
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", status, item.ItemType, item.ItemKey, item.URL)
		if listDiff {
			if err := printDiff(&item); err != nil {
				return err
			}
		}
	}
	return nil
}

type processTarget struct {
	Type       string
	ID         int
	ParentID   int
	Key        string
	URL        string
	ProjectKey string
	Content    string
	Updated    string
	Force      bool
}

type migrateMetadata struct {
	ProjectKey  string `json:"project_key"`
	ProjectName string `json:"project_name"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type migrateItem struct {
	ItemType      string                         `json:"item_type"`
	ItemID        int                            `json:"item_id"`
	ParentID      int                            `json:"parent_id,omitempty"`
	ItemKey       string                         `json:"item_key"`
	URL           string                         `json:"url"`
	ProjectKey    string                         `json:"project_key"`
	FetchedAt     string                         `json:"fetched_at"`
	UpdatedAt     string                         `json:"updated_at"`
	DetectedMode  markdown.Mode                  `json:"detected_mode"`
	Score         int                            `json:"score"`
	Rules         []markdown.RuleID              `json:"rules,omitempty"`
	Warnings      map[markdown.WarningType]int   `json:"warnings,omitempty"`
	WarningLines  map[markdown.WarningType][]int `json:"warning_lines,omitempty"`
	Changed       bool                           `json:"changed"`
	InputHash     string                         `json:"input_hash"`
	OutputHash    string                         `json:"output_hash"`
	OriginalPath  string                         `json:"original_path,omitempty"`
	ConvertedPath string                         `json:"converted_path,omitempty"`
	ConvertForce  bool                           `json:"convert_force"`
	Applied       bool                           `json:"applied"`
	AppliedAt     string                         `json:"applied_at,omitempty"`
	ApplyError    string                         `json:"apply_error,omitempty"`
	RollbackAt    string                         `json:"rollback_at,omitempty"`
	RollbackError string                         `json:"rollback_error,omitempty"`
}

type lockInfo struct {
	PID  int    `json:"pid"`
	Time string `json:"time"`
	Cmd  string `json:"cmd"`
}

type currentItem struct {
	Content string
	Updated string
}

func migrationDir(projectKey string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd: %w", err)
	}
	return filepath.Join(cwd, migrationBaseDir, projectKey), nil
}

func ensureMetadata(dir, projectKey, projectName string) error {
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

func convertItem(target processTarget, baseDir string) (migrateItem, bool, error) {
	result := markdown.Convert(target.Content, markdown.ConvertOptions{
		Force:      target.Force,
		ItemType:   target.Type,
		ItemID:     target.ID,
		ParentID:   target.ParentID,
		ProjectKey: target.ProjectKey,
		ItemKey:    target.Key,
		URL:        target.URL,
	})

	changed := result.Output != target.Content
	item := migrateItem{
		ItemType:     target.Type,
		ItemID:       target.ID,
		ParentID:     target.ParentID,
		ItemKey:      target.Key,
		URL:          target.URL,
		ProjectKey:   target.ProjectKey,
		FetchedAt:    time.Now().Format(time.RFC3339),
		UpdatedAt:    target.Updated,
		DetectedMode: result.Mode,
		Score:        result.Score,
		Rules:        result.Rules,
		Warnings:     result.Warnings,
		WarningLines: result.WarningLines,
		Changed:      changed,
		InputHash:    hashHex(target.Content),
		OutputHash:   hashHex(result.Output),
		ConvertForce: target.Force,
	}

	if changed {
		originalPath, convertedPath, err := writeMigrationFiles(baseDir, target, target.Content, result.Output)
		if err != nil {
			return migrateItem{}, false, err
		}
		item.OriginalPath = originalPath
		item.ConvertedPath = convertedPath
	}

	return item, changed, nil
}

func writeMigrationFiles(baseDir string, target processTarget, original, converted string) (string, string, error) {
	originalPath := filepath.Join(baseDir, "originals", target.Type, safePath(target.Key), "content.md")
	convertedPath := filepath.Join(baseDir, "converted", target.Type, safePath(target.Key), "content.md")

	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		return "", "", fmt.Errorf("create backup dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(convertedPath), 0o755); err != nil {
		return "", "", fmt.Errorf("create converted dir: %w", err)
	}
	if err := os.WriteFile(originalPath, []byte(original), 0o644); err != nil {
		return "", "", fmt.Errorf("write original: %w", err)
	}
	if err := os.WriteFile(convertedPath, []byte(converted), 0o644); err != nil {
		return "", "", fmt.Errorf("write converted: %w", err)
	}
	return originalPath, convertedPath, nil
}

func writeConvertedFile(path string, content string) error {
	if path == "" {
		return errors.New("converted path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create converted dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write converted: %w", err)
	}
	return nil
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
		return &currentItem{Content: optStringValue(issue.Description), Updated: optStringValue(issue.Updated)}, nil
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
		return &currentItem{Content: comment.Content, Updated: comment.Updated}, nil
	case "wiki":
		wiki, err := client.GetWiki(ctx, item.ItemID)
		if err != nil {
			return nil, err
		}
		return &currentItem{Content: wiki.Content, Updated: wiki.Updated}, nil
	default:
		return nil, fmt.Errorf("unknown item type: %s", item.ItemType)
	}
}

func applyItem(ctx context.Context, client *api.Client, item *migrateItem, content string) error {
	switch item.ItemType {
	case "issue":
		_, err := client.UpdateIssue(ctx, item.ItemKey, &api.UpdateIssueInput{Description: &content})
		return err
	case "comment":
		parts := strings.Split(item.ItemKey, "#comment-")
		if len(parts) != 2 {
			return fmt.Errorf("invalid comment key: %s", item.ItemKey)
		}
		commentID, err := parseInt(parts[1])
		if err != nil {
			return fmt.Errorf("invalid comment id: %w", err)
		}
		_, err = client.UpdateComment(ctx, parts[0], commentID, content)
		return err
	case "wiki":
		_, err := client.UpdateWiki(ctx, item.ItemID, &api.UpdateWikiInput{Content: &content})
		return err
	default:
		return fmt.Errorf("unknown item type: %s", item.ItemType)
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

func printChangeSummary(item migrateItem) {
	warnings := formatWarnings(item.Warnings)
	lines := formatWarningLines(item.WarningLines)
	rules := formatRules(item.Rules)
	fmt.Printf("%s %s changed rules=%s warnings=%s lines=%s\n", item.ItemType, item.ItemKey, rules, warnings, lines)
}

func printDiff(item *migrateItem) error {
	if item.OriginalPath == "" || item.ConvertedPath == "" {
		return nil
	}
	if _, err := exec.LookPath("diff"); err != nil {
		original, err := os.ReadFile(item.OriginalPath)
		if err != nil {
			return fmt.Errorf("read original for diff: %w", err)
		}
		converted, err := os.ReadFile(item.ConvertedPath)
		if err != nil {
			return fmt.Errorf("read converted for diff: %w", err)
		}
		fmt.Printf("--- %s\n+++ %s\n", item.OriginalPath, item.ConvertedPath)
		fmt.Println("<<<<< ORIGINAL")
		fmt.Println(string(original))
		fmt.Println(">>>>> CONVERTED")
		fmt.Println(string(converted))
		return nil
	}

	cmd := exec.Command("diff", "-u", item.OriginalPath, item.ConvertedPath)
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

func applyConversion(item *migrateItem, content string) (string, bool, error) {
	result := markdown.Convert(content, markdown.ConvertOptions{
		Force:      item.ConvertForce,
		ItemType:   item.ItemType,
		ItemID:     item.ItemID,
		ParentID:   item.ParentID,
		ProjectKey: item.ProjectKey,
		ItemKey:    item.ItemKey,
		URL:        item.URL,
	})

	changed := result.Output != content
	item.DetectedMode = result.Mode
	item.Score = result.Score
	item.Rules = result.Rules
	item.Warnings = result.Warnings
	item.WarningLines = result.WarningLines
	item.OutputHash = hashHex(result.Output)

	if item.ConvertedPath == "" && item.OriginalPath != "" {
		base := filepath.Dir(filepath.Dir(item.OriginalPath))
		item.ConvertedPath = filepath.Join(base, "converted", item.ItemType, safePath(item.ItemKey), "content.md")
	}
	if item.ConvertedPath != "" {
		if err := writeConvertedFile(item.ConvertedPath, result.Output); err != nil {
			return "", false, err
		}
	}

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
	return allowed[strings.ToLower(itemType)]
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

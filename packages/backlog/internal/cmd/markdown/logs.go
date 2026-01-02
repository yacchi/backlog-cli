package markdown

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/markdown"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var (
	logsLimit int
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show markdown conversion logs",
	Long: `Show markdown conversion logs stored in the cache.

Examples:
  backlog markdown logs
  backlog markdown logs --limit 20
  backlog markdown logs -o json`,
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().IntVar(&logsLimit, "limit", 50, "Maximum number of log entries to show")
}

func runLogs(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	cacheDir, err := cfg.GetCacheDir()
	if err != nil {
		return fmt.Errorf("failed to resolve cache dir: %w", err)
	}

	path := filepath.Join(cacheDir, "markdown", "events.jsonl")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			ui.Info("No markdown log file found: %s", path)
			return nil
		}
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer func() { _ = file.Close() }()

	entries, err := readEntries(file, logsLimit)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		ui.Info("No markdown logs found")
		return nil
	}

	profile := cfg.CurrentProfile()
	if profile.Output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}

	printEntries(entries)
	return nil
}

func readEntries(file *os.File, limit int) ([]markdown.CacheEntry, error) {
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	entries := make([]markdown.CacheEntry, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry markdown.CacheEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("failed to parse log entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	if limit <= 0 || len(entries) <= limit {
		return entries, nil
	}
	return entries[len(entries)-limit:], nil
}

func printEntries(entries []markdown.CacheEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].TS.Before(entries[j].TS)
	})

	for _, entry := range entries {
		warnings := formatWarnings(entry.Warnings)
		label := fmt.Sprintf("%s #%d", entry.ItemType, entry.ItemID)
		if entry.ItemKey != "" {
			label = fmt.Sprintf("%s %s", entry.ItemType, entry.ItemKey)
		}
		lineInfo := formatWarningLines(entry.WarningLines)
		fmt.Printf("%s %s score=%d warnings=%s lines=%s\n", entry.TS.Format("2006-01-02 15:04:05"), label, entry.Score, warnings, lineInfo)
		if entry.URL != "" {
			fmt.Printf("URL: %s\n", entry.URL)
		}
	}
}

func formatWarnings(warnings map[markdown.WarningType]int) string {
	if len(warnings) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(warnings))
	for k := range warnings {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		count := warnings[markdown.WarningType(key)]
		parts = append(parts, fmt.Sprintf("%s=%d", key, count))
	}
	return strings.Join(parts, ", ")
}

func formatWarningLines(lines map[markdown.WarningType][]int) string {
	if len(lines) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(lines))
	for k := range lines {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		lineNums := lines[markdown.WarningType(key)]
		if len(lineNums) == 0 {
			continue
		}
		sort.Ints(lineNums)
		parts = append(parts, fmt.Sprintf("%s:%s", key, joinInts(lineNums)))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func joinInts(values []int) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, fmt.Sprintf("%d", v))
	}
	return strings.Join(out, "/")
}

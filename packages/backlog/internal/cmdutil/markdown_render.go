package cmdutil

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/markdown"
)

// RenderMarkdownContent converts content and optionally prints warnings and caches.
func RenderMarkdownContent(content string, opts MarkdownViewOptions, itemType string, itemID int, parentID int, projectKey string, itemKey string, url string, attachments []string, warnWriter io.Writer) (string, error) {
	unsafeRules := map[markdown.RuleID]bool{}
	for _, rule := range opts.UnsafeRules {
		unsafeRules[markdown.RuleID(rule)] = true
	}
	result := markdown.Convert(content, markdown.ConvertOptions{
		ItemType:        itemType,
		ItemID:          itemID,
		ParentID:        parentID,
		ProjectKey:      projectKey,
		ItemKey:         itemKey,
		URL:             url,
		AttachmentNames: attachments,
		UnsafeRules:     unsafeRules,
	})

	output := result.Output
	if opts.Warn {
		writeWarningSummary(warnWriter, result)
	}

	if opts.Cache {
		entry := markdown.BuildCacheEntry(result, content, output, opts.CacheExcerpt, opts.CacheRaw)
		if err := markdown.AppendCache(entry, opts.CacheDir); err != nil {
			return output, err
		}
	}

	return output, nil
}

func writeWarningSummary(w io.Writer, result markdown.ConvertResult) {
	if w == nil {
		return
	}
	if len(result.Warnings) == 0 {
		return
	}
	pairs := warningPairs(result.Warnings)
	if len(pairs) == 0 {
		return
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "---")
	_, _ = fmt.Fprintln(w, "Markdown Warning Summary")
	_, _ = fmt.Fprintf(w, "- item_type: %s\n", result.ItemType)
	_, _ = fmt.Fprintf(w, "- item_id: %d\n", result.ItemID)
	_, _ = fmt.Fprintf(w, "- detected_mode: %s\n", result.Mode)
	_, _ = fmt.Fprintf(w, "- score: %d\n", result.Score)
	_, _ = fmt.Fprintf(w, "- warnings: %s\n", strings.Join(pairs, ", "))
}

func warningPairs(warnings map[markdown.WarningType]int) []string {
	if len(warnings) == 0 {
		return nil
	}
	keys := make([]string, 0, len(warnings))
	for k := range warnings {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		count := warnings[markdown.WarningType(key)]
		if count <= 0 {
			continue
		}
		pairs = append(pairs, fmt.Sprintf("%s=%d", key, count))
	}
	return pairs
}

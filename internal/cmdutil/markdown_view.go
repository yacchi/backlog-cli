package cmdutil

import (
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/config"
)

// MarkdownViewOptions holds resolved options for markdown view conversion.
type MarkdownViewOptions struct {
	Enable       bool
	Raw          bool
	Warn         bool
	Cache        bool
	CacheRaw     bool
	CacheExcerpt int
	CacheDir     string
	UnsafeRules  []string
}

// ResolveMarkdownViewOptions resolves markdown view flags and config.
func ResolveMarkdownViewOptions(cmd *cobra.Command, display *config.ResolvedDisplay, cacheDir string) MarkdownViewOptions {
	opts := MarkdownViewOptions{
		Enable:       display.MarkdownView,
		Warn:         display.MarkdownWarn,
		Cache:        display.MarkdownCache,
		CacheRaw:     display.MarkdownCacheRaw,
		CacheExcerpt: display.MarkdownCacheExcerpt,
		CacheDir:     cacheDir,
		UnsafeRules:  display.MarkdownUnsafeRules,
	}

	if cmd.Flags().Changed("markdown") {
		if v, err := cmd.Flags().GetBool("markdown"); err == nil {
			opts.Enable = v
		}
	}
	if cmd.Flags().Changed("raw") {
		if v, err := cmd.Flags().GetBool("raw"); err == nil {
			opts.Raw = v
		}
	}
	if cmd.Flags().Changed("markdown-warn") {
		if v, err := cmd.Flags().GetBool("markdown-warn"); err == nil {
			opts.Warn = v
		}
	}
	if cmd.Flags().Changed("markdown-cache") {
		if v, err := cmd.Flags().GetBool("markdown-cache"); err == nil {
			opts.Cache = v
		}
	}

	if opts.Raw {
		opts.Enable = false
	}

	return opts
}

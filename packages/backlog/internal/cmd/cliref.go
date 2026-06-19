package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Flags with the same name and semantics as gh CLI.
// In --diff mode these are omitted from commands under ghEquivGroups.
var ghKnownFlags = map[string]bool{
	"assignee":      true,
	"author":        true,
	"base":          true,
	"body":          true,
	"body-file":     true,
	"comment":       true,
	"delete-branch": true,
	"editor":        true,
	"field":         true,
	"head":          true,
	"include":       true,
	"input":         true,
	"limit":         true,
	"method":        true,
	"milestone":     true,
	"output":        true,
	"raw-field":     true,
	"repo":          true,
	"reviewer":      true,
	"search":        true,
	"silent":        true,
	"state":         true,
	"title":         true,
	"web":           true,
	"yes":           true,
}

// Top-level command groups that have gh equivalents.
var ghEquivGroups = map[string]bool{
	"issue": true,
	"pr":    true,
	"api":   true,
}

var (
	cliRefExclude string
	cliRefDiff    bool
)

var cliRefCmd = &cobra.Command{
	Use:    "cli-ref",
	Short:  "Output CLI reference for MCP tool descriptions",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		w := cmd.OutOrStdout()

		var excludeSet map[string]bool
		if cliRefExclude != "" {
			excludeSet = make(map[string]bool)
			for _, name := range strings.Split(cliRefExclude, ",") {
				excludeSet[strings.TrimSpace(name)] = true
			}
		}

		writeCliReference(w, rootCmd, excludeSet, cliRefDiff)
		return nil
	},
}

func init() {
	cliRefCmd.Flags().StringVar(&cliRefExclude, "exclude", "",
		"Comma-separated top-level command groups to exclude")
	cliRefCmd.Flags().BoolVar(&cliRefDiff, "diff", false,
		"Show only Backlog-specific flags (omit flags common with gh CLI)")
	rootCmd.AddCommand(cliRefCmd)
}

func writeCliReference(w io.Writer, root *cobra.Command, exclude map[string]bool, diff bool) {
	_, _ = fmt.Fprintln(w, "# Backlog CLI Reference")

	if diff {
		_, _ = fmt.Fprintln(w, "NOTE: This CLI follows gh (GitHub CLI) conventions. Flags common with gh (--body, --title, --assignee, --limit, --state, --web, etc.) are omitted below. Only Backlog-specific flags and differences are listed.")
	}

	writeGlobalFlags(w, root, diff)
	writeCommandTree(w, root, "", exclude, diff)
}

func writeGlobalFlags(w io.Writer, root *cobra.Command, diff bool) {
	// Global flags like --json, --jq, --format match gh and are well-known.
	// In diff mode, only show Backlog-specific global flags.
	ghGlobalFlags := map[string]bool{
		"json": true, "jq": true, "format": true, "no-color": true,
	}

	_, _ = fmt.Fprintln(w, "\n## Global Flags")
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		if diff && ghGlobalFlags[f.Name] {
			return
		}
		writeFlag(w, f)
	})
}

func writeCommandTree(w io.Writer, cmd *cobra.Command, prefix string, exclude map[string]bool, diff bool) {
	for _, child := range cmd.Commands() {
		if child.Hidden || child.Name() == "help" || child.Name() == "completion" {
			continue
		}

		topGroup := child.Name()
		if prefix == "" && exclude[topGroup] {
			continue
		}

		fullName := child.Name()
		if prefix != "" {
			fullName = prefix + " " + child.Name()
		}

		subs := visibleSubcommands(child)
		if len(subs) > 0 && !child.Runnable() {
			writeCommandTree(w, child, fullName, exclude, diff)
			continue
		}

		usageArgs := extractUsageArgs(child.Use)
		desc := child.Short
		if usageArgs != "" {
			_, _ = fmt.Fprintf(w, "\n## %s %s — %s\n", fullName, usageArgs, desc)
		} else {
			_, _ = fmt.Fprintf(w, "\n## %s — %s\n", fullName, desc)
		}

		if child.Long != "" && child.Long != child.Short {
			_, _ = fmt.Fprintf(w, "%s\n", child.Long)
		}

		inGhGroup := diff && isInGhGroup(fullName)
		writeLocalFlags(w, child, inGhGroup)

		if len(subs) > 0 {
			writeCommandTree(w, child, fullName, exclude, diff)
		}
	}
}

func isInGhGroup(fullName string) bool {
	group := strings.SplitN(fullName, " ", 2)[0]
	return ghEquivGroups[group]
}

func extractUsageArgs(use string) string {
	parts := strings.SplitN(use, " ", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

// writeLocalFlags safely enumerates local flags, recovering from panics
// caused by shorthand conflicts during Cobra's persistent flag merge.
func writeLocalFlags(w io.Writer, cmd *cobra.Command, skipGhFlags bool) {
	defer func() { _ = recover() }()

	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		if skipGhFlags && ghKnownFlags[f.Name] {
			return
		}
		writeFlag(w, f)
	})
}

func writeFlag(w io.Writer, f *pflag.Flag) {
	var b strings.Builder

	b.WriteString("  ")
	if f.Shorthand != "" {
		b.WriteByte('-')
		b.WriteString(f.Shorthand)
		b.WriteString(", ")
	}
	b.WriteString("--")
	b.WriteString(f.Name)

	typeName := f.Value.Type()
	switch typeName {
	case "bool":
		// no type annotation
	case "stringArray", "stringSlice":
		b.WriteString(" STR[]")
	case "string":
		b.WriteString(" STR")
		if f.NoOptDefVal != "" {
			b.WriteByte('?')
		}
	case "int":
		b.WriteString(" INT")
	default:
		b.WriteByte(' ')
		b.WriteString(strings.ToUpper(typeName))
	}

	if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" && f.DefValue != "[]" {
		b.WriteString(" [")
		b.WriteString(f.DefValue)
		b.WriteByte(']')
	}

	b.WriteString(": ")
	b.WriteString(f.Usage)
	b.WriteByte('\n')

	_, _ = fmt.Fprint(w, b.String())
}

func visibleSubcommands(cmd *cobra.Command) []*cobra.Command {
	var result []*cobra.Command
	for _, c := range cmd.Commands() {
		if !c.Hidden && c.Name() != "help" {
			result = append(result, c)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})
	return result
}

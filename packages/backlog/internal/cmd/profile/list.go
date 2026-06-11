package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured profiles",
	Long: `List all configured profiles with their space, domain, and primary status.

Examples:
  backlog profile list
  backlog profile list --output json`,
	Aliases: []string{"ls"},
	RunE:    runList,
}

type profileEntry struct {
	Name    string `json:"name"`
	Space   string `json:"space"`
	Domain  string `json:"domain"`
	Primary bool   `json:"primary,omitempty"`
	Active  bool   `json:"active,omitempty"`
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	profiles := cfg.Profiles()
	activeProfile := cfg.GetActiveProfile()

	var entries []profileEntry
	for name, p := range profiles {
		entries = append(entries, profileEntry{
			Name:    name,
			Space:   p.Space,
			Domain:  p.Domain,
			Primary: p.Primary,
			Active:  name == activeProfile,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	resolved := cfg.Resolved()
	profile := resolved.GetProfile(activeProfile)
	if profile != nil && profile.Output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tSPACE\tPRIMARY\tACTIVE")
	for _, e := range entries {
		host := ""
		if e.Space != "" {
			host = e.Space
		}
		primary := ""
		if e.Primary {
			primary = "*"
		}
		active := ""
		if e.Active {
			active = "*"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Name, host, primary, active)
	}
	return w.Flush()
}

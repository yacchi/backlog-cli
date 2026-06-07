package cmdutil

import (
	"errors"
	"fmt"
	"strings"
)

// NonInteractiveFlagError formats a consistent non-interactive usage error.
func NonInteractiveFlagError(summary, helpCommand string, hints ...string) error {
	lines := []string{summary}
	if len(hints) > 0 {
		lines = append(lines, "", strings.Join(hints, "\n"))
	}
	lines = append(lines, "Or set BACKLOG_ASSUME_YES=1 to skip all confirmation prompts.")
	if helpCommand != "" {
		lines = append(lines, "", fmt.Sprintf("Run '%s --help' for usage.", helpCommand))
	}
	return errors.New(strings.Join(lines, "\n"))
}

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
	if helpCommand != "" {
		lines = append(lines, "", fmt.Sprintf("Run '%s --help' for usage.", helpCommand))
	}
	return errors.New(strings.Join(lines, "\n"))
}

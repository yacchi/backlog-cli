package domain

import (
	"fmt"
	"strings"
)

// SplitDomain splits "space.backlog.jp" into space and backlog domain.
func SplitDomain(value string) (space, backlogDomain string) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return value, ""
}

// SplitAllowedDomain validates and splits allowed_domain into space and domain.
func SplitAllowedDomain(value string) (string, string, error) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid allowed_domain: %s", value)
	}
	return parts[0], parts[1], nil
}

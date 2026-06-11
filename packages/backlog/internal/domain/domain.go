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

// NormalizeSpace は space と domain を統合した spaceHost を返す。
// space が既にドットを含む場合はそのまま返す（新形式）。
// space がドットを含まず domain が非空の場合は結合する（旧形式からの移行）。
// どちらも空の場合は空文字列を返す。
func NormalizeSpace(space, domain string) string {
	if space == "" {
		return ""
	}
	if strings.Contains(space, ".") {
		return space
	}
	if domain != "" {
		return space + "." + domain
	}
	return space
}

// SpaceID は spaceHost からスペースID部分を返す。
// "mycompany.backlog.jp" → "mycompany"
func SpaceID(spaceHost string) string {
	id, _ := SplitDomain(spaceHost)
	return id
}

// SpaceDomain は spaceHost からドメイン部分を返す。
// "mycompany.backlog.jp" → "backlog.jp"
func SpaceDomain(spaceHost string) string {
	_, d := SplitDomain(spaceHost)
	return d
}

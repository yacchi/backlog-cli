package markdown

import (
	"regexp"
	"strings"
)

var (
	reColorMacro      = regexp.MustCompile(`&color\([^)]*\)\s*\{`)
	reTableHeaderH    = regexp.MustCompile(`(?m)\|.*\|h\s*$`)
	reTableHeaderCell = regexp.MustCompile(`(?m)\|~`)
	reTableCellMerge  = regexp.MustCompile(`\|\|`)
	reThumbnailMacro  = regexp.MustCompile(`#thumbnail\(`)
	reUnknownHash     = regexp.MustCompile(`#([a-zA-Z0-9_+-]+)\([^\)]*\)`)
	reUnknownBrace    = regexp.MustCompile(`\{/?([a-zA-Z0-9_+-]+)(?::[^}]*)?\}`)
)

var (
	allowedHashMacros  = map[string]struct{}{"attach": {}, "image": {}, "thumbnail": {}, "rev": {}, "contents": {}}
	allowedBraceMacros = map[string]struct{}{"code": {}, "quote": {}}
)

// CollectWarnings analyzes input and returns warning counts.
func CollectWarnings(input string) map[WarningType]int {
	warnings := map[WarningType]int{}

	addCount := func(w WarningType, count int) {
		if count <= 0 {
			return
		}
		warnings[w] += count
	}

	addCount(WarningColorMacro, len(reColorMacro.FindAllStringIndex(input, -1)))
	addCount(WarningTableHeaderH, len(reTableHeaderH.FindAllStringIndex(input, -1)))
	addCount(WarningTableHeaderCell, len(reTableHeaderCell.FindAllStringIndex(input, -1)))
	addCount(WarningTableCellMerge, len(reTableCellMerge.FindAllStringIndex(input, -1)))
	addCount(WarningThumbnailMacro, len(reThumbnailMacro.FindAllStringIndex(input, -1)))

	for _, match := range reUnknownHash.FindAllStringSubmatch(input, -1) {
		if len(match) < 2 {
			continue
		}
		name := strings.ToLower(match[1])
		if _, ok := allowedHashMacros[name]; ok {
			continue
		}
		warnings[WarningUnknownHashMacro]++
	}

	for _, match := range reUnknownBrace.FindAllStringSubmatch(input, -1) {
		if len(match) < 2 {
			continue
		}
		name := strings.ToLower(match[1])
		if _, ok := allowedBraceMacros[name]; ok {
			continue
		}
		warnings[WarningUnknownBrace]++
	}

	return warnings
}

// AddWarning increments a warning counter.
func AddWarning(warnings map[WarningType]int, warning WarningType) {
	if warnings == nil {
		return
	}
	warnings[warning]++
}

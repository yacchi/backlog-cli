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
func CollectWarningsWithLines(input string) (map[WarningType]int, map[WarningType][]int) {
	warnings := map[WarningType]int{}
	lines := map[WarningType][]int{}

	addCount := func(w WarningType, count int) {
		if count <= 0 {
			return
		}
		warnings[w] += count
	}
	addLine := func(w WarningType, line int) {
		existing := lines[w]
		if len(existing) > 0 && existing[len(existing)-1] == line {
			return
		}
		lines[w] = append(existing, line)
	}

	for idx, line := range strings.Split(input, "\n") {
		lineNo := idx + 1
		if count := len(reColorMacro.FindAllStringIndex(line, -1)); count > 0 {
			addCount(WarningColorMacro, count)
			addLine(WarningColorMacro, lineNo)
		}
		if count := len(reTableHeaderH.FindAllStringIndex(line, -1)); count > 0 {
			addCount(WarningTableHeaderH, count)
			addLine(WarningTableHeaderH, lineNo)
		}
		if count := len(reTableHeaderCell.FindAllStringIndex(line, -1)); count > 0 {
			addCount(WarningTableHeaderCell, count)
			addLine(WarningTableHeaderCell, lineNo)
		}
		if count := len(reTableCellMerge.FindAllStringIndex(line, -1)); count > 0 {
			addCount(WarningTableCellMerge, count)
			addLine(WarningTableCellMerge, lineNo)
		}
		if count := len(reThumbnailMacro.FindAllStringIndex(line, -1)); count > 0 {
			addCount(WarningThumbnailMacro, count)
			addLine(WarningThumbnailMacro, lineNo)
		}

		for _, match := range reUnknownHash.FindAllStringSubmatch(line, -1) {
			if len(match) < 2 {
				continue
			}
			name := strings.ToLower(match[1])
			if _, ok := allowedHashMacros[name]; ok {
				continue
			}
			warnings[WarningUnknownHashMacro]++
			addLine(WarningUnknownHashMacro, lineNo)
		}

		for _, match := range reUnknownBrace.FindAllStringSubmatch(line, -1) {
			if len(match) < 2 {
				continue
			}
			name := strings.ToLower(match[1])
			if _, ok := allowedBraceMacros[name]; ok {
				continue
			}
			warnings[WarningUnknownBrace]++
			addLine(WarningUnknownBrace, lineNo)
		}
	}

	return warnings, lines
}

func CollectWarnings(input string) map[WarningType]int {
	warnings, _ := CollectWarningsWithLines(input)
	return warnings
}

// AddWarning increments a warning counter.
func AddWarning(warnings map[WarningType]int, warning WarningType) {
	if warnings == nil {
		return
	}
	warnings[warning]++
}

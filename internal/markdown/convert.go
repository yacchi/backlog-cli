package markdown

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	reHeading  = regexp.MustCompile(`(?m)^(\*+)\s+(\S.*)$`)
	reTOC      = regexp.MustCompile(`(?m)^#contents\s*$`)
	reListPlus = regexp.MustCompile(`(?m)^\+\s+`)
	reDashList = regexp.MustCompile(`^(\s*)(-+)\s*(\S.*)$`)
	reOrdered  = regexp.MustCompile(`^\s*\d+\.\s+\S`)

	reQuoteBlock = regexp.MustCompile(`(?s)\{quote\}(.*?)\{/quote\}`)
	reCodeBlock  = regexp.MustCompile(`(?s)\{code(?::([a-zA-Z0-9_+-]+))?\}(.*?)\{/code\}`)

	reBacklogLink     = regexp.MustCompile(`\[\[([^\]]+?)\]\]`)
	reBold            = regexp.MustCompile(`''([^']+?)''`)
	reItalic          = regexp.MustCompile(`'''([^']+?)'''`)
	reStrike          = regexp.MustCompile(`%%([^%]+?)%%`)
	reInlineCodeToken = regexp.MustCompile("`[^`]*`")
	reQuoteLine       = regexp.MustCompile(`^\s*>`)
	reTableHeaderMark = regexp.MustCompile(`\|h\s*$`)
)

// Convert converts Backlog markdown to GFM when needed.
func Convert(input string, opts ConvertOptions) ConvertResult {
	result := ConvertResult{
		Output:     input,
		ItemType:   opts.ItemType,
		ItemID:     opts.ItemID,
		ParentID:   opts.ParentID,
		ProjectKey: opts.ProjectKey,
		ItemKey:    opts.ItemKey,
		URL:        opts.URL,
	}

	detect := Detect(input)
	result.Mode = detect.Mode
	result.Score = detect.Score
	result.Warnings, result.WarningLines = CollectWarningsWithLines(input)

	if result.Mode != ModeBacklog && !(result.Mode == ModeUnknown && opts.Force) {
		return result
	}

	lineBreak := opts.LineBreak
	if lineBreak == "" {
		lineBreak = "<br>"
	}

	converted, rules, warnings := applyConversion(input, lineBreak, result.Warnings)
	result.Output = converted
	result.Rules = rules
	result.Warnings = warnings

	return result
}

func applyConversion(input, lineBreak string, warnings map[WarningType]int) (string, []RuleID, map[WarningType]int) {
	rules := []RuleID{}
	content := input

	// Extract quote blocks first to avoid conversions inside quotes.
	content, quoteTokens := replaceBlocks(content, reQuoteBlock, "QUOTE", func(groups []string) string {
		body := strings.Trim(groups[1], "\n")
		lines := strings.Split(body, "\n")
		for i, line := range lines {
			if line == "" {
				lines[i] = ">"
				continue
			}
			lines[i] = "> " + line
		}
		rules = appendRule(rules, RuleQuoteBlock)
		return strings.Join(lines, "\n")
	})

	// Replace quote lines to avoid conversions inside.
	content, quoteLineTokens := replaceQuoteLines(content)

	// Extract code blocks.
	content, codeTokens := replaceBlocks(content, reCodeBlock, "CODE", func(groups []string) string {
		lang := strings.TrimSpace(groups[1])
		body := strings.Trim(groups[2], "\n")
		if !strings.Contains(body, "\n") {
			rules = appendRule(rules, RuleCodeBlock)
			return inlineCodeWithLang(lang, body)
		}
		head := "```"
		if lang != "" {
			head += lang
		}
		block := head + "\n" + body + "\n```"
		rules = appendRule(rules, RuleCodeBlock)
		return block
	})

	// Replace inline code with tokens to avoid conversions inside.
	content, codeInlineTokens := replaceInlineTokens(content, reInlineCodeToken)

	// Headings
	content = reHeading.ReplaceAllStringFunc(content, func(match string) string {
		parts := reHeading.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		level := len(parts[1])
		rules = appendRule(rules, RuleHeadingAsterisk)
		return strings.Repeat("#", level) + " " + parts[2]
	})

	// TOC
	content = reTOC.ReplaceAllString(content, "[toc]")
	if reTOC.MatchString(input) {
		rules = appendRule(rules, RuleTOC)
	}

	// Lists
	content = reListPlus.ReplaceAllString(content, "1. ")
	if reListPlus.MatchString(input) {
		rules = appendRule(rules, RuleListPlus)
	}
	content, listChanged := convertDashLists(content)
	if listChanged {
		rules = appendRule(rules, RuleListDashSpace)
	}

	// Tables
	content, tableChanged := convertTables(content)
	if tableChanged {
		rules = appendRule(rules, RuleTableSeparator)
	}

	// Inline conversions
	content = reBacklogLink.ReplaceAllStringFunc(content, func(match string) string {
		parts := reBacklogLink.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		label, url, ok, warn := parseBacklogLink(parts[1])
		if warn {
			AddWarning(warnings, WarningWikiLinkAmbig)
			return match
		}
		if !ok {
			return match
		}
		rules = appendRule(rules, RuleBacklogLink)
		if label == "" {
			return "<" + url + ">"
		}
		return "[" + label + "](" + url + ")"
	})

	italicChanged := false
	content = reItalic.ReplaceAllStringFunc(content, func(match string) string {
		parts := reItalic.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		italicChanged = true
		rules = appendRule(rules, RuleEmphasisItalic)
		return "*" + parts[1] + "*"
	})

	boldChanged := false
	content = reBold.ReplaceAllStringFunc(content, func(match string) string {
		parts := reBold.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		boldChanged = true
		rules = appendRule(rules, RuleEmphasisBold)
		return "**" + parts[1] + "**"
	})

	strikeChanged := false
	content = reStrike.ReplaceAllStringFunc(content, func(match string) string {
		parts := reStrike.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		strikeChanged = true
		rules = appendRule(rules, RuleStrikethrough)
		return "~~" + parts[1] + "~~"
	})

	if boldChanged || italicChanged {
		if strings.Contains(content, "''") {
			AddWarning(warnings, WarningEmphasisAmbig)
		}
	}

	if strikeChanged && strings.Contains(content, "%%") {
		AddWarning(warnings, WarningEmphasisAmbig)
	}

	if strings.Contains(content, "&br;") {
		content = strings.ReplaceAll(content, "&br;", lineBreak)
		rules = appendRule(rules, RuleLineBreak)
	}

	// Restore inline code tokens.
	content = restoreTokens(content, codeInlineTokens)
	// Restore quote/code blocks.
	content = restoreTokens(content, quoteLineTokens)
	content = restoreTokens(content, quoteTokens)
	content = restoreTokens(content, codeTokens)

	return content, rules, warnings
}

func replaceBlocks(input string, re *regexp.Regexp, prefix string, fn func(groups []string) string) (string, map[string]string) {
	matches := re.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return input, nil
	}
	var b strings.Builder
	b.Grow(len(input))
	replacements := make(map[string]string, len(matches))
	last := 0
	for i, m := range matches {
		start, end := m[0], m[1]
		b.WriteString(input[last:start])
		groups := make([]string, len(m)/2)
		for g := 0; g < len(groups); g++ {
			idx := g * 2
			if m[idx] == -1 {
				groups[g] = ""
				continue
			}
			groups[g] = input[m[idx]:m[idx+1]]
		}
		token := fmt.Sprintf("{{BACKLOG_MD_%s_%d}}", prefix, i)
		replacements[token] = fn(groups)
		b.WriteString(token)
		last = end
	}
	b.WriteString(input[last:])
	return b.String(), replacements
}

func replaceInlineTokens(input string, re *regexp.Regexp) (string, map[string]string) {
	matches := re.FindAllStringIndex(input, -1)
	if len(matches) == 0 {
		return input, nil
	}
	var b strings.Builder
	b.Grow(len(input))
	replacements := make(map[string]string, len(matches))
	last := 0
	for i, m := range matches {
		start, end := m[0], m[1]
		b.WriteString(input[last:start])
		token := fmt.Sprintf("{{BACKLOG_MD_INLINE_%d}}", i)
		replacements[token] = input[start:end]
		b.WriteString(token)
		last = end
	}
	b.WriteString(input[last:])
	return b.String(), replacements
}

func replaceQuoteLines(input string) (string, map[string]string) {
	lines := strings.Split(input, "\n")
	replacements := make(map[string]string, 0)
	for i, line := range lines {
		if !reQuoteLine.MatchString(line) {
			continue
		}
		token := fmt.Sprintf("{{BACKLOG_MD_QUOTE_LINE_%d}}", i)
		replacements[token] = line
		lines[i] = token
	}
	if len(replacements) == 0 {
		return input, nil
	}
	return strings.Join(lines, "\n"), replacements
}

func convertTables(input string) (string, bool) {
	lines := strings.Split(input, "\n")
	if len(lines) == 0 {
		return input, false
	}

	changed := false
	out := make([]string, 0, len(lines)+1)
	inGFMTable := false
	for i, line := range lines {
		if isTableSeparator(line) {
			out = append(out, line)
			inGFMTable = true
			continue
		}
		if isTableRow(line) {
			if inGFMTable {
				out = append(out, line)
				continue
			}

			prevIsTable := i > 0 && (isTableRow(lines[i-1]) || isTableSeparator(lines[i-1]))
			nextIsSeparator := i+1 < len(lines) && isTableSeparator(lines[i+1])
			normalized := normalizeTableRow(line)
			if normalized != line {
				changed = true
			}
			if !prevIsTable && !nextIsSeparator && i > 0 && strings.TrimSpace(lines[i-1]) != "" {
				out = append(out, "")
				changed = true
			}
			out = append(out, normalized)

			if !prevIsTable {
				if !nextIsSeparator {
					sep := tableSeparatorLine(normalized)
					if sep != "" {
						out = append(out, sep)
						changed = true
					}
				}
			}
			continue
		}

		inGFMTable = false
		out = append(out, line)
	}
	return strings.Join(out, "\n"), changed
}

func convertDashLists(input string) (string, bool) {
	lines := strings.Split(input, "\n")
	if len(lines) == 0 {
		return input, false
	}
	changed := false
	out := make([]string, 0, len(lines)+1)
	for i, line := range lines {
		converted, isList := normalizeDashListLine(line)
		if converted != line {
			changed = true
		}
		out = append(out, converted)

		if !isList {
			continue
		}
		nextLine := ""
		if i+1 < len(lines) {
			nextLine = lines[i+1]
		}
		if nextLine == "" {
			continue
		}
		if isListLine(nextLine) {
			continue
		}
		out = append(out, "")
		changed = true
	}
	return strings.Join(out, "\n"), changed
}

func normalizeDashListLine(line string) (string, bool) {
	match := reDashList.FindStringSubmatch(line)
	if len(match) < 4 {
		return line, false
	}
	leading := match[1]
	dashes := match[2]
	text := strings.TrimSpace(match[3])
	if text == "" {
		return line, false
	}
	depth := len(dashes) - 1
	if depth < 0 {
		depth = 0
	}
	indent := leading + strings.Repeat("  ", depth)
	return indent + "- " + text, true
}

func isListLine(line string) bool {
	if reDashList.MatchString(line) {
		return true
	}
	if reListPlus.MatchString(line) {
		return true
	}
	return reOrdered.MatchString(strings.TrimLeft(line, " \t"))
}

func isTableRow(line string) bool {
	if isTableSeparator(line) {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, "|") {
		return false
	}
	return tableColumnCount(trimmed) >= 2
}

func isTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "|") {
		trimmed = strings.TrimPrefix(trimmed, "|")
	}
	if strings.HasSuffix(trimmed, "|") {
		trimmed = strings.TrimSuffix(trimmed, "|")
	}
	if !strings.Contains(trimmed, "|") {
		return false
	}
	for _, part := range strings.Split(trimmed, "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		for _, r := range part {
			if r != '-' && r != ':' {
				return false
			}
		}
	}
	return true
}

func normalizeTableRow(line string) string {
	normalized := reTableHeaderMark.ReplaceAllString(line, "|")
	normalized = strings.ReplaceAll(normalized, "|~", "|")
	trimmed := strings.TrimSpace(normalized)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	cells := strings.Split(trimmed, "|")
	for i, cell := range cells {
		cells[i] = strings.TrimSpace(cell)
	}
	return "| " + strings.Join(cells, " | ") + " |"
}

func tableColumnCount(line string) int {
	trimmed := strings.TrimSpace(line)
	trimmed = reTableHeaderMark.ReplaceAllString(trimmed, "|")
	if strings.HasPrefix(trimmed, "|") {
		trimmed = trimmed[1:]
	}
	if strings.HasSuffix(trimmed, "|") {
		trimmed = trimmed[:len(trimmed)-1]
	}
	parts := strings.Split(trimmed, "|")
	if len(parts) < 2 {
		return 0
	}
	return len(parts)
}

func tableSeparatorLine(row string) string {
	columns := tableColumnCount(row)
	if columns < 2 {
		return ""
	}
	parts := make([]string, columns)
	for i := range parts {
		parts[i] = "---"
	}
	return "| " + strings.Join(parts, " | ") + " |"
}

func inlineCode(text string) string {
	if text == "" {
		return "``"
	}
	maxRun := 0
	current := 0
	for _, r := range text {
		if r == '`' {
			current++
			if current > maxRun {
				maxRun = current
			}
			continue
		}
		current = 0
	}
	fence := strings.Repeat("`", maxRun+1)
	return fence + text + fence
}

func inlineCodeWithLang(lang, body string) string {
	lang = strings.TrimSpace(lang)
	body = strings.TrimSpace(body)
	if lang == "" {
		return inlineCode(body)
	}
	if body == "" {
		return inlineCode(lang + ":")
	}
	return inlineCode(lang + ": " + body)
}

func restoreTokens(input string, tokens map[string]string) string {
	if len(tokens) == 0 {
		return input
	}
	out := input
	for token, value := range tokens {
		out = strings.ReplaceAll(out, token, value)
	}
	return out
}

func appendRule(rules []RuleID, rule RuleID) []RuleID {
	for _, r := range rules {
		if r == rule {
			return rules
		}
	}
	return append(rules, rule)
}

func parseBacklogLink(content string) (label string, url string, ok bool, warn bool) {
	if strings.Contains(content, ">") {
		parts := strings.SplitN(content, ">", 2)
		label = strings.TrimSpace(parts[0])
		url = strings.TrimSpace(parts[1])
		if isURL(url) {
			return label, url, true, false
		}
		return "", "", false, true
	}
	if strings.Contains(content, ":") && !startsWithURLScheme(content) {
		parts := strings.SplitN(content, ":", 2)
		label = strings.TrimSpace(parts[0])
		url = strings.TrimSpace(parts[1])
		if isURL(url) {
			return label, url, true, false
		}
		return "", "", false, true
	}

	trimmed := strings.TrimSpace(content)
	if isURL(trimmed) {
		return "", trimmed, true, false
	}
	if isIssueKey(trimmed) {
		return "", "", false, false
	}
	return "", "", false, true
}

func startsWithURLScheme(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:")
}

func isURL(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:")
}

var issueKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]+-\d+$`)

func isIssueKey(value string) bool {
	return issueKeyPattern.MatchString(strings.TrimSpace(value))
}

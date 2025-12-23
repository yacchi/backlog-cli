package markdown

import (
	"regexp"
	"strings"
)

var (
	reStrongBacklogSignals = []*regexp.Regexp{
		regexp.MustCompile(`\{code(?::[^}]+)?\}`),
		regexp.MustCompile(`\{quote\}`),
		regexp.MustCompile(`(?m)^#contents\s*$`),
		regexp.MustCompile(`&br;`),
		regexp.MustCompile(`&color\([^)]*\)\s*\{`),
		regexp.MustCompile(`%%[^%]+%%`),
		regexp.MustCompile(`'''[^']+'''`),
		regexp.MustCompile(`''[^']+''`),
		regexp.MustCompile(`\[\[[^\]]+\]\]`),
	}
	reWeakBacklogSignals = []*regexp.Regexp{
		regexp.MustCompile(`(?m)^\*{1,3}\s+\S`),
		regexp.MustCompile(`(?m)^\+\s+\S`),
		regexp.MustCompile(`(?m)\|.*\|h\s*$`),
	}
	gfmSignals = []*regexp.Regexp{
		regexp.MustCompile(`(?m)^\s*[-*+]\s+\[[ xX]\]\s+`),
		regexp.MustCompile("(?m)^```+"),
		regexp.MustCompile(`(?m)^\s*\|?.*\|.*\n\s*\|?\s*:?-{3,}:?\s*\|`),
		regexp.MustCompile(`~~[^~]+~~`),
		regexp.MustCompile(`<https?://[^>]+>`),
		regexp.MustCompile(`(?m)^\s*\[[^\]]+\]:\s+\S+`),
	}
	reInlineCode = regexp.MustCompile("`[^`]*`")
	reFencedCode = regexp.MustCompile("(?s)```.*?```")
)

// Detect determines markdown mode based on heuristics.
func Detect(input string) DetectResult {
	filtered := stripCodeRegions(input)

	score := 0
	for _, re := range reStrongBacklogSignals {
		if re.MatchString(filtered) {
			score += 2
		}
	}
	for _, re := range reWeakBacklogSignals {
		if re.MatchString(filtered) {
			score++
		}
	}
	for _, re := range gfmSignals {
		if re.MatchString(filtered) {
			score -= 2
		}
	}

	mode := ModeUnknown
	switch {
	case score >= 2:
		mode = ModeBacklog
	case score <= -1:
		mode = ModeMarkdown
	default:
		mode = ModeUnknown
	}

	return DetectResult{Mode: mode, Score: score}
}

func stripCodeRegions(input string) string {
	out := reFencedCode.ReplaceAllString(input, "")
	out = reInlineCode.ReplaceAllString(out, "")
	return strings.TrimSpace(out)
}

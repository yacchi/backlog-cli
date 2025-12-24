package markdown

import "testing"

func TestCollectWarnings(t *testing.T) {
	input := "&color(red) { x }\n| a |h\n|~ b|\n||\n#thumbnail(1)\n#foo(1)\n{bar}"
	warnings := CollectWarnings(input)

	assertWarn(t, warnings, WarningColorMacro, 1)
	assertWarn(t, warnings, WarningTableHeaderH, 1)
	assertWarn(t, warnings, WarningTableHeaderCell, 1)
	assertWarn(t, warnings, WarningTableCellMerge, 1)
	assertWarn(t, warnings, WarningThumbnailMacro, 1)
	assertWarn(t, warnings, WarningUnknownHashMacro, 1)
	assertWarn(t, warnings, WarningUnknownBrace, 1)
}

func assertWarn(t *testing.T, warnings map[WarningType]int, key WarningType, min int) {
	t.Helper()
	if warnings[key] < min {
		t.Fatalf("expected warning %s >= %d, got %d", key, min, warnings[key])
	}
}

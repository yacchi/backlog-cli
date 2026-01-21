package summary

import (
	"testing"
)

func TestFormatInput(t *testing.T) {
	tests := []struct {
		name   string
		issues []IssueInput
		want   string
	}{
		{
			name:   "empty",
			issues: []IssueInput{},
			want:   "",
		},
		{
			name: "single issue without comments",
			issues: []IssueInput{
				{
					Key:         "PROJ-001",
					Description: "This is a test issue",
				},
			},
			want: "=== PROJ-001 ===\nThis is a test issue\n",
		},
		{
			name: "single issue with title",
			issues: []IssueInput{
				{
					Key:         "PROJ-001",
					Title:       "ログイン機能の不具合",
					Description: "This is a test issue",
				},
			},
			want: "=== PROJ-001 ===\nタイトル: ログイン機能の不具合\nThis is a test issue\n",
		},
		{
			name: "single issue with comments",
			issues: []IssueInput{
				{
					Key:         "PROJ-001",
					Title:       "バグ修正",
					Description: "This is a test issue",
					Comments:    []string{"Comment 1", "Comment 2"},
				},
			},
			want: "=== PROJ-001 ===\nタイトル: バグ修正\nThis is a test issue\n---コメント---\nComment 1\nComment 2\n",
		},
		{
			name: "multiple issues",
			issues: []IssueInput{
				{
					Key:         "PROJ-001",
					Title:       "First title",
					Description: "First issue",
				},
				{
					Key:         "PROJ-002",
					Title:       "Second title",
					Description: "Second issue",
				},
			},
			want: "=== PROJ-001 ===\nタイトル: First title\nFirst issue\n\n=== PROJ-002 ===\nタイトル: Second title\nSecond issue\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatInput(tt.issues)
			if got != tt.want {
				t.Errorf("FormatInput() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   map[string]string
	}{
		{
			name:   "empty",
			output: "",
			want:   map[string]string{},
		},
		{
			name:   "single issue",
			output: "=== PROJ-001 ===\nThis is the summary",
			want: map[string]string{
				"PROJ-001": "This is the summary",
			},
		},
		{
			name:   "multiple issues",
			output: "=== PROJ-001 ===\nFirst summary\n\n=== PROJ-002 ===\nSecond summary",
			want: map[string]string{
				"PROJ-001": "First summary",
				"PROJ-002": "Second summary",
			},
		},
		{
			name:   "multiline summary",
			output: "=== PROJ-001 ===\nLine 1\nLine 2\nLine 3",
			want: map[string]string{
				"PROJ-001": "Line 1\nLine 2\nLine 3",
			},
		},
		{
			name:   "with extra whitespace",
			output: "=== PROJ-001 ===\n  Summary with spaces  \n",
			want: map[string]string{
				"PROJ-001": "Summary with spaces",
			},
		},
		{
			name:   "key with underscore",
			output: "=== OPTAGE_OPERATION-501 ===\nSummary for underscore key\n\n=== QS2-1007 ===\nNormal key summary",
			want: map[string]string{
				"OPTAGE_OPERATION-501": "Summary for underscore key",
				"QS2-1007":             "Normal key summary",
			},
		},
		{
			name:   "key line with trailing spaces",
			output: "=== QS2-666 ===  \nコールIDコピー機能をメイン画面に移動。\n\n=== QS2-970 ===  \nユーザー権限ごとにUI表示を制御。",
			want: map[string]string{
				"QS2-666": "コールIDコピー機能をメイン画面に移動。",
				"QS2-970": "ユーザー権限ごとにUI表示を制御。",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseOutput(tt.output)
			if len(got) != len(tt.want) {
				t.Errorf("ParseOutput() returned %d items, want %d", len(got), len(tt.want))
				return
			}
			for key, wantVal := range tt.want {
				gotVal, ok := got[key]
				if !ok {
					t.Errorf("ParseOutput() missing key %q", key)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("ParseOutput()[%q] = %q, want %q", key, gotVal, wantVal)
				}
			}
		})
	}
}

func TestParseSingleOutput(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		expectedKey string
		want        string
	}{
		{
			name:        "with key marker",
			output:      "=== PROJ-001 ===\nSummary text",
			expectedKey: "PROJ-001",
			want:        "Summary text",
		},
		{
			name:        "without key marker",
			output:      "Direct summary without markers",
			expectedKey: "PROJ-001",
			want:        "Direct summary without markers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSingleOutput(tt.output, tt.expectedKey)
			if got != tt.want {
				t.Errorf("ParseSingleOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTruncateSummary(t *testing.T) {
	tests := []struct {
		name    string
		summary string
		maxLen  int
		want    string
	}{
		{
			name:    "no truncation needed",
			summary: "Short",
			maxLen:  10,
			want:    "Short",
		},
		{
			name:    "truncation needed",
			summary: "This is a long summary text",
			maxLen:  10,
			want:    "This is a ...",
		},
		{
			name:    "newline conversion",
			summary: "Line 1\nLine 2",
			maxLen:  50,
			want:    "Line 1 Line 2",
		},
		{
			name:    "zero max length",
			summary: "Any text",
			maxLen:  0,
			want:    "Any text",
		},
		{
			name:    "unicode characters",
			summary: "日本語のテスト文字列です",
			maxLen:  5,
			want:    "日本語のテ...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateSummary(tt.summary, tt.maxLen)
			if got != tt.want {
				t.Errorf("TruncateSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

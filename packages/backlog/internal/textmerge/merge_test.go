package textmerge

import "testing"

func TestThreeWayMerge(t *testing.T) {
	tests := []struct {
		name       string
		base       string
		ours       string
		theirs     string
		wantClean  bool
		wantResult string
	}{
		{
			name:       "no changes",
			base:       "line1\nline2\nline3",
			ours:       "line1\nline2\nline3",
			theirs:     "line1\nline2\nline3",
			wantClean:  true,
			wantResult: "line1\nline2\nline3",
		},
		{
			name:       "only ours changed",
			base:       "line1\nline2\nline3",
			ours:       "line1\nmodified\nline3",
			theirs:     "line1\nline2\nline3",
			wantClean:  true,
			wantResult: "line1\nmodified\nline3",
		},
		{
			name:       "only theirs changed",
			base:       "line1\nline2\nline3",
			ours:       "line1\nline2\nline3",
			theirs:     "line1\nline2\nchanged",
			wantClean:  true,
			wantResult: "line1\nline2\nchanged",
		},
		{
			name:       "both changed different regions",
			base:       "line1\nline2\nline3\nline4\nline5",
			ours:       "CHANGED1\nline2\nline3\nline4\nline5",
			theirs:     "line1\nline2\nline3\nline4\nCHANGED5",
			wantClean:  true,
			wantResult: "CHANGED1\nline2\nline3\nline4\nCHANGED5",
		},
		{
			name:       "both changed same region same way",
			base:       "line1\nline2\nline3",
			ours:       "line1\nSAME\nline3",
			theirs:     "line1\nSAME\nline3",
			wantClean:  true,
			wantResult: "line1\nSAME\nline3",
		},
		{
			name:       "both changed same region differently - conflict",
			base:       "line1\nline2\nline3",
			ours:       "line1\nOURS\nline3",
			theirs:     "line1\nTHEIRS\nline3",
			wantClean:  false,
			wantResult: "line1\n<<<<<<< ours\nOURS\n=======\nTHEIRS\n>>>>>>> theirs\nline3",
		},
		{
			name:       "ours inserted lines",
			base:       "line1\nline3",
			ours:       "line1\nline2\nline3",
			theirs:     "line1\nline3",
			wantClean:  true,
			wantResult: "line1\nline2\nline3",
		},
		{
			name:       "theirs deleted lines",
			base:       "line1\nline2\nline3",
			ours:       "line1\nline2\nline3",
			theirs:     "line1\nline3",
			wantClean:  true,
			wantResult: "line1\nline3",
		},
		{
			name:       "ours inserted and theirs modified different region",
			base:       "aaa\nbbb\nccc",
			ours:       "aaa\nNEW\nbbb\nccc",
			theirs:     "aaa\nbbb\nCCC",
			wantClean:  true,
			wantResult: "aaa\nNEW\nbbb\nCCC",
		},
		{
			name:       "empty base",
			base:       "",
			ours:       "hello",
			theirs:     "world",
			wantClean:  false,
			wantResult: "<<<<<<< ours\nhello\n=======\nworld\n>>>>>>> theirs",
		},
		{
			name:       "empty base both same",
			base:       "",
			ours:       "same",
			theirs:     "same",
			wantClean:  true,
			wantResult: "same",
		},
		{
			name:       "ours appended",
			base:       "line1\nline2",
			ours:       "line1\nline2\nline3",
			theirs:     "line1\nline2",
			wantClean:  true,
			wantResult: "line1\nline2\nline3",
		},
		{
			name:       "both appended different content",
			base:       "line1",
			ours:       "line1\nours-append",
			theirs:     "line1\ntheirs-append",
			wantClean:  false,
			wantResult: "line1\n<<<<<<< ours\nours-append\n=======\ntheirs-append\n>>>>>>> theirs",
		},
		{
			name:       "multiline changes in different sections",
			base:       "# Header\n\n## Section A\ncontent-a\n\n## Section B\ncontent-b",
			ours:       "# Header\n\n## Section A\nupdated-a\n\n## Section B\ncontent-b",
			theirs:     "# Header\n\n## Section A\ncontent-a\n\n## Section B\nupdated-b",
			wantClean:  true,
			wantResult: "# Header\n\n## Section A\nupdated-a\n\n## Section B\nupdated-b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ThreeWayMerge(tt.base, tt.ours, tt.theirs)
			if result.Clean != tt.wantClean {
				t.Errorf("Clean = %v, want %v", result.Clean, tt.wantClean)
			}
			if result.Content != tt.wantResult {
				t.Errorf("Content mismatch:\ngot:  %q\nwant: %q", result.Content, tt.wantResult)
			}
		})
	}
}

func TestThreeWayMergeConflictCount(t *testing.T) {
	base := "a\nb\nc\nd\ne"
	ours := "A\nb\nc\nd\nE"
	theirs := "X\nb\nc\nd\nY"

	result := ThreeWayMerge(base, ours, theirs)
	if result.Clean {
		t.Fatal("expected conflicts")
	}
	if len(result.Conflicts) != 2 {
		t.Errorf("got %d conflicts, want 2", len(result.Conflicts))
	}
}

func TestComputeHunks(t *testing.T) {
	tests := []struct {
		name  string
		base  []string
		mod   []string
		hunks []Hunk
	}{
		{
			name:  "no diff",
			base:  []string{"a", "b", "c"},
			mod:   []string{"a", "b", "c"},
			hunks: nil,
		},
		{
			name: "single replacement",
			base: []string{"a", "b", "c"},
			mod:  []string{"a", "X", "c"},
			hunks: []Hunk{
				{BaseStart: 1, BaseEnd: 2, Lines: []string{"X"}},
			},
		},
		{
			name: "insertion",
			base: []string{"a", "c"},
			mod:  []string{"a", "b", "c"},
			hunks: []Hunk{
				{BaseStart: 1, BaseEnd: 1, Lines: []string{"b"}},
			},
		},
		{
			name: "deletion",
			base: []string{"a", "b", "c"},
			mod:  []string{"a", "c"},
			hunks: []Hunk{
				{BaseStart: 1, BaseEnd: 2, Lines: nil},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeHunks(tt.base, tt.mod)
			if len(got) != len(tt.hunks) {
				t.Fatalf("got %d hunks, want %d", len(got), len(tt.hunks))
			}
			for i := range got {
				if got[i].BaseStart != tt.hunks[i].BaseStart || got[i].BaseEnd != tt.hunks[i].BaseEnd {
					t.Errorf("hunk[%d] base range: got [%d,%d), want [%d,%d)",
						i, got[i].BaseStart, got[i].BaseEnd,
						tt.hunks[i].BaseStart, tt.hunks[i].BaseEnd)
				}
				if !slicesEqual(got[i].Lines, tt.hunks[i].Lines) {
					t.Errorf("hunk[%d] lines: got %v, want %v", i, got[i].Lines, tt.hunks[i].Lines)
				}
			}
		})
	}
}

func TestLCS(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want [][2]int
	}{
		{"empty", nil, nil, nil},
		{"identical", []string{"a", "b"}, []string{"a", "b"}, [][2]int{{0, 0}, {1, 1}}},
		{"no common", []string{"a"}, []string{"b"}, nil},
		{"partial", []string{"a", "b", "c"}, []string{"a", "c"}, [][2]int{{0, 0}, {2, 1}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lcs(tt.a, tt.b)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

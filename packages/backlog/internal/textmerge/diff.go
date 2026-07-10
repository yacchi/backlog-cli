package textmerge

import "strings"

// Hunk represents a contiguous region where mod differs from base.
type Hunk struct {
	BaseStart int      // inclusive start line in base
	BaseEnd   int      // exclusive end line in base
	Lines     []string // replacement lines in the modified text
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

// lcs computes the longest common subsequence of two string slices.
// Returns pairs of matching indices (a[i], b[j]).
func lcs(a, b []string) [][2]int {
	m, n := len(a), len(b)
	if m == 0 || n == 0 {
		return nil
	}

	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	result := make([][2]int, 0, dp[m][n])
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			result = append(result, [2]int{i - 1, j - 1})
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}
	// reverse
	for l, r := 0, len(result)-1; l < r; l, r = l+1, r-1 {
		result[l], result[r] = result[r], result[l]
	}
	return result
}

// computeHunks returns the diff between base and mod as a list of hunks.
func computeHunks(base, mod []string) []Hunk {
	matches := lcs(base, mod)

	var hunks []Hunk
	bi, mi := 0, 0

	for _, m := range matches {
		if bi < m[0] || mi < m[1] {
			hunks = append(hunks, Hunk{
				BaseStart: bi,
				BaseEnd:   m[0],
				Lines:     copySlice(mod[mi:m[1]]),
			})
		}
		bi = m[0] + 1
		mi = m[1] + 1
	}

	if bi < len(base) || mi < len(mod) {
		hunks = append(hunks, Hunk{
			BaseStart: bi,
			BaseEnd:   len(base),
			Lines:     copySlice(mod[mi:]),
		})
	}

	return hunks
}

func copySlice(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	c := make([]string, len(s))
	copy(c, s)
	return c
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

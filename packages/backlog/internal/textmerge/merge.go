package textmerge

// Conflict describes a region where ours and theirs made incompatible changes.
type Conflict struct {
	BaseStart  int      // line in base where conflict starts (0-based)
	BaseEnd    int      // exclusive end line in base
	OursLines  []string // what ours has for this region
	TheirLines []string // what theirs has for this region
}

// MergeResult holds the output of a three-way merge.
type MergeResult struct {
	Content   string     // merged text (with conflict markers if any)
	Clean     bool       // true if no conflicts
	Conflicts []Conflict // non-empty when Clean is false
}

// ThreeWayMerge performs a line-based three-way merge.
//
// base is the common ancestor. ours and theirs are two divergent versions.
// If both sides changed different regions, changes are combined automatically.
// If both sides changed the same region identically, one copy is kept.
// If both sides changed the same region differently, conflict markers are inserted.
func ThreeWayMerge(base, ours, theirs string) MergeResult {
	baseLines := splitLines(base)
	oursLines := splitLines(ours)
	theirsLines := splitLines(theirs)

	oHunks := computeHunks(baseLines, oursLines)
	tHunks := computeHunks(baseLines, theirsLines)

	var result []string
	var conflicts []Conflict

	bi := 0        // current position in base
	oi, ti := 0, 0 // current index in oHunks, tHunks

	for oi < len(oHunks) || ti < len(tHunks) {
		var oh, th *Hunk
		if oi < len(oHunks) {
			oh = &oHunks[oi]
		}
		if ti < len(tHunks) {
			th = &tHunks[ti]
		}

		switch {
		case oh != nil && th != nil && hunksOverlap(oh, th):
			mergeStart := min(oh.BaseStart, th.BaseStart)
			mergeEnd := max(oh.BaseEnd, th.BaseEnd)

			// Collect all overlapping hunks from both sides into one merged region
			oStart, oEnd := oh.BaseStart, oh.BaseEnd
			tStart, tEnd := th.BaseStart, th.BaseEnd
			var oLines, tLines []string
			oLines = append(oLines, oh.Lines...)
			tLines = append(tLines, th.Lines...)
			oi++
			ti++

			// Absorb any additional overlapping hunks
			for oi < len(oHunks) && oHunks[oi].BaseStart < mergeEnd {
				h := &oHunks[oi]
				oLines = append(oLines, baseLines[oEnd:h.BaseStart]...)
				oLines = append(oLines, h.Lines...)
				oEnd = h.BaseEnd
				mergeEnd = max(mergeEnd, h.BaseEnd)
				oi++
			}
			for ti < len(tHunks) && tHunks[ti].BaseStart < mergeEnd {
				h := &tHunks[ti]
				tLines = append(tLines, baseLines[tEnd:h.BaseStart]...)
				tLines = append(tLines, h.Lines...)
				tEnd = h.BaseEnd
				mergeEnd = max(mergeEnd, h.BaseEnd)
				ti++
			}

			// Include unchanged base lines around the hunks within the merge region
			oFull := buildRegion(baseLines, oStart, oEnd, mergeStart, mergeEnd, oLines)
			tFull := buildRegion(baseLines, tStart, tEnd, mergeStart, mergeEnd, tLines)

			result = append(result, baseLines[bi:mergeStart]...)
			if slicesEqual(oFull, tFull) {
				result = append(result, oFull...)
			} else {
				conflicts = append(conflicts, Conflict{
					BaseStart:  mergeStart,
					BaseEnd:    mergeEnd,
					OursLines:  oFull,
					TheirLines: tFull,
				})
				result = append(result, "<<<<<<< ours")
				result = append(result, oFull...)
				result = append(result, "=======")
				result = append(result, tFull...)
				result = append(result, ">>>>>>> theirs")
			}
			bi = mergeEnd

		case oh != nil && (th == nil || oh.BaseStart < th.BaseStart):
			result = append(result, baseLines[bi:oh.BaseStart]...)
			result = append(result, oh.Lines...)
			bi = oh.BaseEnd
			oi++

		default:
			result = append(result, baseLines[bi:th.BaseStart]...)
			result = append(result, th.Lines...)
			bi = th.BaseEnd
			ti++
		}
	}

	// Remaining unchanged base lines
	if bi < len(baseLines) {
		result = append(result, baseLines[bi:]...)
	}

	return MergeResult{
		Content:   joinLines(result),
		Clean:     len(conflicts) == 0,
		Conflicts: conflicts,
	}
}

func hunksOverlap(a, b *Hunk) bool {
	// Pure insertions (zero-length base range) at the same position overlap
	if a.BaseStart == a.BaseEnd && b.BaseStart == b.BaseEnd {
		return a.BaseStart == b.BaseStart
	}
	return a.BaseStart < b.BaseEnd && b.BaseStart < a.BaseEnd
}

// buildRegion constructs the full content for one side within [regionStart, regionEnd).
// hunkStart..hunkEnd is the range the hunk replaces, hunkLines are the replacement.
// Lines outside the hunk but inside the region are taken from base.
func buildRegion(base []string, hunkStart, hunkEnd, regionStart, regionEnd int, hunkLines []string) []string {
	var out []string
	if regionStart < hunkStart {
		out = append(out, base[regionStart:hunkStart]...)
	}
	out = append(out, hunkLines...)
	if hunkEnd < regionEnd {
		out = append(out, base[hunkEnd:regionEnd]...)
	}
	return out
}

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/textmerge"
)

// PatchOp represents a single search-and-replace operation.
type PatchOp struct {
	Find    string
	Replace string
}

// SafeUpdateResult holds the result of a safe wiki update.
type SafeUpdateResult struct {
	Wiki       *Wiki
	Merged     bool // true if three-way merge was performed
	PatchCount int  // number of patches applied (for search-and-replace mode)
}

// ConflictError is returned when a three-way merge cannot resolve conflicts.
type ConflictError struct {
	Base       string
	Ours       string
	Theirs     string
	MergedText string // text with conflict markers
	Conflicts  []textmerge.Conflict
	UpdatedBy  string // who made the conflicting update
}

func (e *ConflictError) Error() string {
	n := len(e.Conflicts)
	if e.UpdatedBy != "" {
		return fmt.Sprintf("conflict: %d region(s) conflict with changes by %s", n, e.UpdatedBy)
	}
	return fmt.Sprintf("conflict: %d region(s) could not be automatically merged", n)
}

// SafeUpdateWiki performs a Read-Modify-Write update with conflict detection.
//
// patchFn receives the current wiki content and returns the modified content.
// If the remote content changed between read and write, a three-way merge is
// attempted. If the merge has conflicts, a *ConflictError is returned.
func (c *Client) SafeUpdateWiki(
	ctx context.Context,
	wikiID int,
	patchFn func(current string) (string, error),
) (*SafeUpdateResult, error) {
	// Phase 1: Read
	wiki, err := c.GetWiki(ctx, wikiID)
	if err != nil {
		return nil, fmt.Errorf("failed to read wiki: %w", err)
	}
	base := wiki.Content
	baseUpdated := wiki.Updated
	baseHash := contentHash(base)

	// Phase 2: Apply patch
	ours, err := patchFn(base)
	if err != nil {
		return nil, err
	}

	if ours == base {
		return &SafeUpdateResult{Wiki: wiki}, nil
	}

	// Phase 3: Pre-write conflict check
	wiki2, err := c.GetWiki(ctx, wikiID)
	if err != nil {
		return nil, fmt.Errorf("failed to re-read wiki for conflict check: %w", err)
	}

	theirs := wiki2.Content
	theirsUpdated := wiki2.Updated
	theirsHash := contentHash(theirs)

	if baseUpdated == theirsUpdated && baseHash == theirsHash {
		// No remote changes — safe to write directly
		updated, err := c.UpdateWiki(ctx, wikiID, &UpdateWikiInput{Content: &ours})
		if err != nil {
			return nil, fmt.Errorf("failed to update wiki: %w", err)
		}
		return &SafeUpdateResult{Wiki: updated}, nil
	}

	// Phase 4: Remote changed — attempt three-way merge
	result := textmerge.ThreeWayMerge(base, ours, theirs)
	if !result.Clean {
		updatedBy := ""
		if wiki2.UpdatedUser != nil {
			updatedBy = wiki2.UpdatedUser.Name
		}
		return nil, &ConflictError{
			Base:       base,
			Ours:       ours,
			Theirs:     theirs,
			MergedText: result.Content,
			Conflicts:  result.Conflicts,
			UpdatedBy:  updatedBy,
		}
	}

	// Clean merge — write merged content
	merged := result.Content
	updated, err := c.UpdateWiki(ctx, wikiID, &UpdateWikiInput{Content: &merged})
	if err != nil {
		return nil, fmt.Errorf("failed to update wiki after merge: %w", err)
	}
	return &SafeUpdateResult{Wiki: updated, Merged: true}, nil
}

// PatchFnReplace creates a patchFn that applies search-and-replace operations.
func PatchFnReplace(ops []PatchOp) func(string) (string, error) {
	return func(current string) (string, error) {
		result := current
		for _, op := range ops {
			if !strings.Contains(result, op.Find) {
				return "", fmt.Errorf("patch target not found: %q", op.Find)
			}
			result = strings.Replace(result, op.Find, op.Replace, 1)
		}
		return result, nil
	}
}

// PatchFnAppend creates a patchFn that appends text.
func PatchFnAppend(text string) func(string) (string, error) {
	return func(current string) (string, error) {
		if current == "" {
			return text, nil
		}
		return current + "\n" + text, nil
	}
}

// PatchFnPrepend creates a patchFn that prepends text.
func PatchFnPrepend(text string) func(string) (string, error) {
	return func(current string) (string, error) {
		if current == "" {
			return text, nil
		}
		return text + "\n" + current, nil
	}
}

// PatchFnFullReplace creates a patchFn that replaces the entire content.
// Used with SafeUpdateWiki to get conflict detection on full replacement.
func PatchFnFullReplace(newContent string) func(string) (string, error) {
	return func(_ string) (string, error) {
		return newContent, nil
	}
}

func contentHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

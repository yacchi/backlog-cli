package api

import (
	"context"
	"fmt"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/textmerge"
)

// SafeUpdateIssueDescription performs a Read-Modify-Write update on an issue's
// description with conflict detection and three-way merge.
//
// patchFn receives the current description and returns the modified description.
// If the remote description changed between read and write, a three-way merge is
// attempted. If the merge has conflicts, a *ConflictError is returned.
func (c *Client) SafeUpdateIssueDescription(
	ctx context.Context,
	issueIDOrKey string,
	patchFn func(current string) (string, error),
) (*backlog.Issue, bool, error) {
	// Phase 1: Read (bypass cache)
	issue, err := c.getIssueNoCache(ctx, issueIDOrKey)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read issue: %w", err)
	}
	base := issue.Description.Value
	baseUpdated := issue.Updated.Value
	baseHash := contentHash(base)

	// Phase 2: Apply patch
	ours, err := patchFn(base)
	if err != nil {
		return nil, false, err
	}

	if ours == base {
		return issue, false, nil
	}

	// Phase 3: Pre-write conflict check (bypass cache)
	issue2, err := c.getIssueNoCache(ctx, issueIDOrKey)
	if err != nil {
		return nil, false, fmt.Errorf("failed to re-read issue for conflict check: %w", err)
	}

	theirs := issue2.Description.Value
	theirsUpdated := issue2.Updated.Value
	theirsHash := contentHash(theirs)

	content := ours
	merged := false

	if baseUpdated != theirsUpdated || baseHash != theirsHash {
		// Remote changed — attempt three-way merge
		result := textmerge.ThreeWayMerge(base, ours, theirs)
		if !result.Clean {
			updatedBy := ""
			if issue2.UpdatedUser.IsSet() {
				updatedBy = issue2.UpdatedUser.Value.Name.Value
			}
			return nil, false, &ConflictError{
				Base:       base,
				Ours:       ours,
				Theirs:     theirs,
				MergedText: result.Content,
				Conflicts:  result.Conflicts,
				UpdatedBy:  updatedBy,
			}
		}
		content = result.Content
		merged = true
	}

	updated, err := c.UpdateIssue(ctx, issueIDOrKey, &UpdateIssueInput{
		Description: &content,
	})
	if err != nil {
		return nil, false, fmt.Errorf("failed to update issue: %w", err)
	}
	return updated, merged, nil
}

func (c *Client) getIssueNoCache(ctx context.Context, issueIDOrKey string) (*backlog.Issue, error) {
	return c.backlogClient.GetIssue(ctx, backlog.GetIssueParams{
		IssueIdOrKey: issueIDOrKey,
	})
}

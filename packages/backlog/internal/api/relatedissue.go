package api

import (
	"context"
	"fmt"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
)

// RelatedIssue は関連課題（RELATES）情報。
// 通常の課題情報（Issue）の全フィールドに加えて、関連の種類を示す Type フィールド
// （現在は常に "RELATES"）を持つ。
type RelatedIssue = backlog.RelatedIssue

// GetRelatedIssues は課題に設定されている関連課題の一覧を取得する
func (c *Client) GetRelatedIssues(ctx context.Context, issueIDOrKey string) ([]RelatedIssue, error) {
	res, err := c.backlogClient.GetRelatedIssueList(ctx, backlog.GetRelatedIssueListParams{
		IssueIdOrKey: issueIDOrKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get related issues: %w", err)
	}
	return res, nil
}

// AddRelatedIssue は課題に関連課題を追加する。relatedIssueID には追加したい
// 課題自身の課題ID（数値）を指定する。
func (c *Client) AddRelatedIssue(ctx context.Context, issueIDOrKey string, relatedIssueID int) (*RelatedIssue, error) {
	res, err := c.backlogClient.AddRelatedIssue(ctx,
		backlog.OptAddRelatedIssueReq{
			Set: true,
			Value: backlog.AddRelatedIssueReq{
				RelatedIssueId: relatedIssueID,
			},
		},
		backlog.AddRelatedIssueParams{IssueIdOrKey: issueIDOrKey},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to add related issue: %w", err)
	}
	return res, nil
}

// DeleteRelatedIssue は課題から関連課題の関連付けを解除する。relatedIssueID には
// 関連付けを解除したい課題自身の課題ID（数値）を指定する（関連付け自体を表す
// 別のIDではない）。
func (c *Client) DeleteRelatedIssue(ctx context.Context, issueIDOrKey string, relatedIssueID int) (*RelatedIssue, error) {
	res, err := c.backlogClient.DeleteRelatedIssue(ctx, backlog.DeleteRelatedIssueParams{
		IssueIdOrKey:   issueIDOrKey,
		RelatedIssueId: relatedIssueID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to delete related issue: %w", err)
	}
	return res, nil
}

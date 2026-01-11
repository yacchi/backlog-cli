package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// PullRequest はプルリクエスト
type PullRequest struct {
	ID           int      `json:"id"`
	ProjectID    int      `json:"projectId"`
	RepositoryID int      `json:"repositoryId"`
	Number       int      `json:"number"`
	Summary      string   `json:"summary"`
	Description  string   `json:"description"`
	Base         string   `json:"base"`
	Branch       string   `json:"branch"`
	Status       PRStatus `json:"status"`
	Assignee     *User    `json:"assignee"`
	Issue        *Issue   `json:"issue"`
	BaseCommit   string   `json:"baseCommit"`
	BranchCommit string   `json:"branchCommit"`
	CloseAt      string   `json:"closeAt"`
	MergeAt      string   `json:"mergeAt"`
	CreatedUser  User     `json:"createdUser"`
	Created      string   `json:"created"`
	UpdatedUser  *User    `json:"updatedUser"`
	Updated      string   `json:"updated"`
}

// PRStatus はPRステータス
type PRStatus struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Repository はリポジトリ
type Repository struct {
	ID           int    `json:"id"`
	ProjectID    int    `json:"projectId"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	HookURL      string `json:"hookUrl"`
	HTTPURL      string `json:"httpUrl"`
	SSHURL       string `json:"sshUrl"`
	DisplayOrder int    `json:"displayOrder"`
	PushedAt     string `json:"pushedAt"`
	CreatedUser  User   `json:"createdUser"`
	Created      string `json:"created"`
	UpdatedUser  *User  `json:"updatedUser"`
	Updated      string `json:"updated"`
}

// GetRepositories はリポジトリ一覧を取得する
func (c *Client) GetRepositories(ctx context.Context, projectIDOrKey string) ([]Repository, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s/git/repositories", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var repos []Repository
	if err := DecodeResponse(resp, &repos); err != nil {
		return nil, err
	}

	return repos, nil
}

// PRListOptions はPR一覧取得オプション
type PRListOptions struct {
	StatusIDs      []int
	AssigneeIDs    []int
	IssueIDs       []int
	CreatedUserIDs []int
	Offset         int
	Count          int
}

// ToQuery はクエリパラメータに変換する
func (o *PRListOptions) ToQuery() url.Values {
	q := url.Values{}
	for _, id := range o.StatusIDs {
		q.Add("statusId[]", strconv.Itoa(id))
	}
	for _, id := range o.AssigneeIDs {
		q.Add("assigneeId[]", strconv.Itoa(id))
	}
	for _, id := range o.IssueIDs {
		q.Add("issueId[]", strconv.Itoa(id))
	}
	for _, id := range o.CreatedUserIDs {
		q.Add("createdUserId[]", strconv.Itoa(id))
	}
	if o.Offset > 0 {
		q.Set("offset", strconv.Itoa(o.Offset))
	}
	if o.Count > 0 {
		q.Set("count", strconv.Itoa(o.Count))
	}
	return q
}

// GetPullRequests はプルリクエスト一覧を取得する
func (c *Client) GetPullRequests(ctx context.Context, projectIDOrKey, repoIDOrName string, opts *PRListOptions) ([]PullRequest, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}

	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s/git/repositories/%s/pullRequests", projectIDOrKey, repoIDOrName), query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var prs []PullRequest
	if err := DecodeResponse(resp, &prs); err != nil {
		return nil, err
	}

	return prs, nil
}

// GetPullRequest はプルリクエストを取得する
func (c *Client) GetPullRequest(ctx context.Context, projectIDOrKey, repoIDOrName string, number int) (*PullRequest, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s/git/repositories/%s/pullRequests/%d", projectIDOrKey, repoIDOrName, number), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var pr PullRequest
	if err := DecodeResponse(resp, &pr); err != nil {
		return nil, err
	}

	return &pr, nil
}

// GetPullRequestsCount はプルリクエスト数を取得する
func (c *Client) GetPullRequestsCount(ctx context.Context, projectIDOrKey, repoIDOrName string, opts *PRListOptions) (int, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}

	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s/git/repositories/%s/pullRequests/count", projectIDOrKey, repoIDOrName), query)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Count int `json:"count"`
	}
	if err := DecodeResponse(resp, &result); err != nil {
		return 0, err
	}

	return result.Count, nil
}

// CreatePullRequestInput はPR作成の入力
type CreatePullRequestInput struct {
	Summary         string
	Description     string
	Base            string
	Branch          string
	IssueID         int
	AssigneeID      int
	NotifiedUserIDs []int
}

// CreatePullRequest はプルリクエストを作成する
func (c *Client) CreatePullRequest(ctx context.Context, projectIDOrKey, repoIDOrName string, input *CreatePullRequestInput) (*PullRequest, error) {
	data := url.Values{}
	data.Set("summary", input.Summary)
	data.Set("description", input.Description)
	data.Set("base", input.Base)
	data.Set("branch", input.Branch)
	if input.IssueID > 0 {
		data.Set("issueId", strconv.Itoa(input.IssueID))
	}
	if input.AssigneeID > 0 {
		data.Set("assigneeId", strconv.Itoa(input.AssigneeID))
	}
	for _, id := range input.NotifiedUserIDs {
		data.Add("notifiedUserId[]", strconv.Itoa(id))
	}

	resp, err := c.PostForm(ctx, fmt.Sprintf("/projects/%s/git/repositories/%s/pullRequests", projectIDOrKey, repoIDOrName), data)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var pr PullRequest
	if err := DecodeResponse(resp, &pr); err != nil {
		return nil, err
	}

	return &pr, nil
}

// PRComment はPRコメント
// 注: ChangeLog, Star, Notification型はcomment.go, issue.goで定義済み
type PRComment struct {
	ID            int            `json:"id"`
	Content       string         `json:"content"`
	ChangeLog     []ChangeLog    `json:"changeLog"`
	CreatedUser   User           `json:"createdUser"`
	Created       string         `json:"created"`
	Updated       string         `json:"updated"`
	Stars         []Star         `json:"stars"`
	Notifications []Notification `json:"notifications"`
}

// PRCommentListOptions はPRコメント一覧取得オプション
type PRCommentListOptions struct {
	MinID int
	MaxID int
	Count int
	Order string // "asc" or "desc"
}

// ToQuery はクエリパラメータに変換する
func (o *PRCommentListOptions) ToQuery() url.Values {
	q := url.Values{}
	if o.MinID > 0 {
		q.Set("minId", strconv.Itoa(o.MinID))
	}
	if o.MaxID > 0 {
		q.Set("maxId", strconv.Itoa(o.MaxID))
	}
	if o.Count > 0 {
		q.Set("count", strconv.Itoa(o.Count))
	}
	if o.Order != "" {
		q.Set("order", o.Order)
	}
	return q
}

// GetPullRequestComments はプルリクエストのコメント一覧を取得する
func (c *Client) GetPullRequestComments(ctx context.Context, projectIDOrKey, repoIDOrName string, number int, opts *PRCommentListOptions) ([]PRComment, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}

	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s/git/repositories/%s/pullRequests/%d/comments", projectIDOrKey, repoIDOrName, number), query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var comments []PRComment
	if err := DecodeResponse(resp, &comments); err != nil {
		return nil, err
	}

	return comments, nil
}

// AddPRCommentInput はPRコメント追加の入力
type AddPRCommentInput struct {
	Content         string
	NotifiedUserIDs []int
}

// AddPullRequestComment はプルリクエストにコメントを追加する
func (c *Client) AddPullRequestComment(ctx context.Context, projectIDOrKey, repoIDOrName string, number int, input *AddPRCommentInput) (*PRComment, error) {
	data := url.Values{}
	data.Set("content", input.Content)
	for _, id := range input.NotifiedUserIDs {
		data.Add("notifiedUserId[]", strconv.Itoa(id))
	}

	resp, err := c.PostForm(ctx, fmt.Sprintf("/projects/%s/git/repositories/%s/pullRequests/%d/comments", projectIDOrKey, repoIDOrName, number), data)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var comment PRComment
	if err := DecodeResponse(resp, &comment); err != nil {
		return nil, err
	}

	return &comment, nil
}

// UpdatePullRequestInput はPR更新の入力
type UpdatePullRequestInput struct {
	Summary         *string
	Description     *string
	IssueID         *int
	AssigneeID      *int
	NotifiedUserIDs []int
}

// UpdatePullRequest はプルリクエストを更新する
func (c *Client) UpdatePullRequest(ctx context.Context, projectIDOrKey, repoIDOrName string, number int, input *UpdatePullRequestInput) (*PullRequest, error) {
	data := url.Values{}
	if input.Summary != nil {
		data.Set("summary", *input.Summary)
	}
	if input.Description != nil {
		data.Set("description", *input.Description)
	}
	if input.IssueID != nil {
		if *input.IssueID > 0 {
			data.Set("issueId", strconv.Itoa(*input.IssueID))
		}
	}
	if input.AssigneeID != nil {
		if *input.AssigneeID > 0 {
			data.Set("assigneeId", strconv.Itoa(*input.AssigneeID))
		}
	}
	for _, id := range input.NotifiedUserIDs {
		data.Add("notifiedUserId[]", strconv.Itoa(id))
	}

	resp, err := c.PatchForm(ctx, fmt.Sprintf("/projects/%s/git/repositories/%s/pullRequests/%d", projectIDOrKey, repoIDOrName, number), data)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var pr PullRequest
	if err := DecodeResponse(resp, &pr); err != nil {
		return nil, err
	}

	return &pr, nil
}

// PRStatusOpen はオープン状態
const PRStatusOpen = 1

// PRStatusClosed はクローズ状態
const PRStatusClosed = 2

// PRStatusMerged はマージ済み状態
const PRStatusMerged = 3

// ClosePullRequestInput はPRクローズの入力
type ClosePullRequestInput struct {
	Comment string
}

// ClosePullRequest はプルリクエストをクローズする
func (c *Client) ClosePullRequest(ctx context.Context, projectIDOrKey, repoIDOrName string, number int, input *ClosePullRequestInput) (*PullRequest, error) {
	data := url.Values{}
	data.Set("statusId", strconv.Itoa(PRStatusClosed))
	if input != nil && input.Comment != "" {
		data.Set("comment", input.Comment)
	}

	resp, err := c.PatchForm(ctx, fmt.Sprintf("/projects/%s/git/repositories/%s/pullRequests/%d", projectIDOrKey, repoIDOrName, number), data)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var pr PullRequest
	if err := DecodeResponse(resp, &pr); err != nil {
		return nil, err
	}

	return &pr, nil
}

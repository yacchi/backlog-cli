package api

import (
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
func (c *Client) GetRepositories(projectIDOrKey string) ([]Repository, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/git/repositories", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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
func (c *Client) GetPullRequests(projectIDOrKey, repoIDOrName string, opts *PRListOptions) ([]PullRequest, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}

	resp, err := c.Get(fmt.Sprintf("/projects/%s/git/repositories/%s/pullRequests", projectIDOrKey, repoIDOrName), query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var prs []PullRequest
	if err := DecodeResponse(resp, &prs); err != nil {
		return nil, err
	}

	return prs, nil
}

// GetPullRequest はプルリクエストを取得する
func (c *Client) GetPullRequest(projectIDOrKey, repoIDOrName string, number int) (*PullRequest, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/git/repositories/%s/pullRequests/%d", projectIDOrKey, repoIDOrName, number), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var pr PullRequest
	if err := DecodeResponse(resp, &pr); err != nil {
		return nil, err
	}

	return &pr, nil
}

// GetPullRequestsCount はプルリクエスト数を取得する
func (c *Client) GetPullRequestsCount(projectIDOrKey, repoIDOrName string, opts *PRListOptions) (int, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}

	resp, err := c.Get(fmt.Sprintf("/projects/%s/git/repositories/%s/pullRequests/count", projectIDOrKey, repoIDOrName), query)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		Count int `json:"count"`
	}
	if err := DecodeResponse(resp, &result); err != nil {
		return 0, err
	}

	return result.Count, nil
}

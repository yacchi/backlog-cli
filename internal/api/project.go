package api

import (
	"context"
	"fmt"
	"net/url"
)

// Project はプロジェクト情報
type Project struct {
	ID                                int    `json:"id"`
	ProjectKey                        string `json:"projectKey"`
	Name                              string `json:"name"`
	ChartEnabled                      bool   `json:"chartEnabled"`
	UseResolvedForChart               bool   `json:"useResolvedForChart"`
	SubtaskingEnabled                 bool   `json:"subtaskingEnabled"`
	ProjectLeaderCanEditProjectLeader bool   `json:"projectLeaderCanEditProjectLeader"`
	UseWiki                           bool   `json:"useWiki"`
	UseFileSharing                    bool   `json:"useFileSharing"`
	UseWikiTreeView                   bool   `json:"useWikiTreeView"`
	UseOriginalImageSizeAtWiki        bool   `json:"useOriginalImageSizeAtWiki"`
	TextFormattingRule                string `json:"textFormattingRule"`
	Archived                          bool   `json:"archived"`
	DisplayOrder                      int    `json:"displayOrder"`
	UseDevAttributes                  bool   `json:"useDevAttributes"`
}

// GetProjects はプロジェクト一覧を取得する
func (c *Client) GetProjects(ctx context.Context) ([]Project, error) {
	query := url.Values{}
	query.Set("archived", "false")

	resp, err := c.Get(ctx, "/projects", query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var projects []Project
	if err := DecodeResponse(resp, &projects); err != nil {
		return nil, err
	}

	return projects, nil
}

// GetProject はプロジェクト情報を取得する
func (c *Client) GetProject(ctx context.Context, projectIDOrKey string) (*Project, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var project Project
	if err := DecodeResponse(resp, &project); err != nil {
		return nil, err
	}

	return &project, nil
}

// IssueType は課題種別
type IssueType struct {
	ID                  int    `json:"id"`
	ProjectID           int    `json:"projectId"`
	Name                string `json:"name"`
	Color               string `json:"color"`
	DisplayOrder        int    `json:"displayOrder"`
	TemplateSummary     string `json:"templateSummary"`
	TemplateDescription string `json:"templateDescription"`
}

// CreateIssueTypeInput は種別作成の入力
type CreateIssueTypeInput struct {
	Name                string
	Color               string
	TemplateSummary     string
	TemplateDescription string
}

// UpdateIssueTypeInput は種別更新の入力
type UpdateIssueTypeInput struct {
	Name                *string
	Color               *string
	TemplateSummary     *string
	TemplateDescription *string
}

// GetIssueTypes は課題種別一覧を取得する
func (c *Client) GetIssueTypes(ctx context.Context, projectIDOrKey string) ([]IssueType, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s/issueTypes", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var issueTypes []IssueType
	if err := DecodeResponse(resp, &issueTypes); err != nil {
		return nil, err
	}

	return issueTypes, nil
}

// CreateIssueType は課題種別を作成する
func (c *Client) CreateIssueType(ctx context.Context, projectIDOrKey string, input *CreateIssueTypeInput) (*IssueType, error) {
	data := url.Values{}
	data.Set("name", input.Name)
	data.Set("color", input.Color)
	if input.TemplateSummary != "" {
		data.Set("templateSummary", input.TemplateSummary)
	}
	if input.TemplateDescription != "" {
		data.Set("templateDescription", input.TemplateDescription)
	}

	resp, err := c.PostForm(ctx, fmt.Sprintf("/projects/%s/issueTypes", projectIDOrKey), data)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var issueType IssueType
	if err := DecodeResponse(resp, &issueType); err != nil {
		return nil, err
	}

	return &issueType, nil
}

// UpdateIssueType は課題種別を更新する
func (c *Client) UpdateIssueType(ctx context.Context, projectIDOrKey string, issueTypeID int, input *UpdateIssueTypeInput) (*IssueType, error) {
	data := url.Values{}
	if input.Name != nil {
		data.Set("name", *input.Name)
	}
	if input.Color != nil {
		data.Set("color", *input.Color)
	}
	if input.TemplateSummary != nil {
		data.Set("templateSummary", *input.TemplateSummary)
	}
	if input.TemplateDescription != nil {
		data.Set("templateDescription", *input.TemplateDescription)
	}

	resp, err := c.PatchForm(ctx, fmt.Sprintf("/projects/%s/issueTypes/%d", projectIDOrKey, issueTypeID), data)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var issueType IssueType
	if err := DecodeResponse(resp, &issueType); err != nil {
		return nil, err
	}

	return &issueType, nil
}

// DeleteIssueType は課題種別を削除する
func (c *Client) DeleteIssueType(ctx context.Context, projectIDOrKey string, issueTypeID int, substituteIssueTypeID int) (*IssueType, error) {
	data := url.Values{}
	data.Set("substituteIssueTypeId", fmt.Sprintf("%d", substituteIssueTypeID))

	// DELETE with body - need to use custom request
	resp, err := c.DeleteWithForm(ctx, fmt.Sprintf("/projects/%s/issueTypes/%d", projectIDOrKey, issueTypeID), data)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var issueType IssueType
	if err := DecodeResponse(resp, &issueType); err != nil {
		return nil, err
	}

	return &issueType, nil
}

// Category はカテゴリー
type Category struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	DisplayOrder int    `json:"displayOrder"`
}

// GetCategories はカテゴリー一覧を取得する
func (c *Client) GetCategories(ctx context.Context, projectIDOrKey string) ([]Category, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s/categories", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var categories []Category
	if err := DecodeResponse(resp, &categories); err != nil {
		return nil, err
	}

	return categories, nil
}

// Version はバージョン/マイルストーン
type Version struct {
	ID             int    `json:"id"`
	ProjectID      int    `json:"projectId"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	StartDate      string `json:"startDate"`
	ReleaseDueDate string `json:"releaseDueDate"`
	Archived       bool   `json:"archived"`
	DisplayOrder   int    `json:"displayOrder"`
}

// GetVersions はバージョン一覧を取得する
func (c *Client) GetVersions(ctx context.Context, projectIDOrKey string) ([]Version, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s/versions", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var versions []Version
	if err := DecodeResponse(resp, &versions); err != nil {
		return nil, err
	}

	return versions, nil
}

// Status はステータス
type Status struct {
	ID           int    `json:"id"`
	ProjectID    int    `json:"projectId"`
	Name         string `json:"name"`
	Color        string `json:"color"`
	DisplayOrder int    `json:"displayOrder"`
}

// GetStatuses はステータス一覧を取得する
func (c *Client) GetStatuses(ctx context.Context, projectIDOrKey string) ([]Status, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s/statuses", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var statuses []Status
	if err := DecodeResponse(resp, &statuses); err != nil {
		return nil, err
	}

	return statuses, nil
}

// GetProjectUsers はプロジェクトユーザー一覧を取得する
func (c *Client) GetProjectUsers(ctx context.Context, projectIDOrKey string) ([]User, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s/users", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var users []User
	if err := DecodeResponse(resp, &users); err != nil {
		return nil, err
	}

	return users, nil
}

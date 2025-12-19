package api

import (
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
func (c *Client) GetProjects() ([]Project, error) {
	query := url.Values{}
	query.Set("archived", "false")

	resp, err := c.Get("/projects", query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var projects []Project
	if err := DecodeResponse(resp, &projects); err != nil {
		return nil, err
	}

	return projects, nil
}

// GetProject はプロジェクト情報を取得する
func (c *Client) GetProject(projectIDOrKey string) (*Project, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var project Project
	if err := DecodeResponse(resp, &project); err != nil {
		return nil, err
	}

	return &project, nil
}

// IssueType は課題種別
type IssueType struct {
	ID           int    `json:"id"`
	ProjectID    int    `json:"projectId"`
	Name         string `json:"name"`
	Color        string `json:"color"`
	DisplayOrder int    `json:"displayOrder"`
}

// GetIssueTypes は課題種別一覧を取得する
func (c *Client) GetIssueTypes(projectIDOrKey string) ([]IssueType, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/issueTypes", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var issueTypes []IssueType
	if err := DecodeResponse(resp, &issueTypes); err != nil {
		return nil, err
	}

	return issueTypes, nil
}

// Category はカテゴリー
type Category struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	DisplayOrder int    `json:"displayOrder"`
}

// GetCategories はカテゴリー一覧を取得する
func (c *Client) GetCategories(projectIDOrKey string) ([]Category, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/categories", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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
func (c *Client) GetVersions(projectIDOrKey string) ([]Version, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/versions", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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
func (c *Client) GetStatuses(projectIDOrKey string) ([]Status, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/statuses", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var statuses []Status
	if err := DecodeResponse(resp, &statuses); err != nil {
		return nil, err
	}

	return statuses, nil
}

// GetProjectUsers はプロジェクトユーザー一覧を取得する
func (c *Client) GetProjectUsers(projectIDOrKey string) ([]User, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/users", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var users []User
	if err := DecodeResponse(resp, &users); err != nil {
		return nil, err
	}

	return users, nil
}

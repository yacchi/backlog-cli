package api

import (
	"context"
	"fmt"
)

// GitRepository はGitリポジトリ情報
type GitRepository struct {
	ID           int    `json:"id"`
	ProjectID    int    `json:"projectId"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	HookURL      string `json:"hookUrl"`
	HTTPURL      string `json:"httpUrl"`
	SSHURL       string `json:"sshUrl"`
	DisplayOrder int    `json:"displayOrder"`
	PushedAt     string `json:"pushedAt"`
	CreatedUser  *User  `json:"createdUser"`
	Created      string `json:"created"`
	UpdatedUser  *User  `json:"updatedUser"`
	Updated      string `json:"updated"`
}

// GetGitRepositories はGitリポジトリ一覧を取得する
func (c *Client) GetGitRepositories(ctx context.Context, projectIDOrKey string) ([]GitRepository, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s/git/repositories", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var repos []GitRepository
	if err := DecodeResponse(resp, &repos); err != nil {
		return nil, err
	}

	return repos, nil
}

// GetGitRepository はGitリポジトリを取得する
func (c *Client) GetGitRepository(ctx context.Context, projectIDOrKey, repoIDOrName string) (*GitRepository, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s/git/repositories/%s", projectIDOrKey, repoIDOrName), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var repo GitRepository
	if err := DecodeResponse(resp, &repo); err != nil {
		return nil, err
	}

	return &repo, nil
}

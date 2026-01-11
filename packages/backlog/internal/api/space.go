package api

import (
	"context"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
)

// GetSpace はスペース情報を取得する
func (c *Client) GetSpace(ctx context.Context) (*backlog.Space, error) {
	return c.backlogClient.GetSpace(ctx)
}

// GetUsers はスペース内の全ユーザー一覧を取得する
func (c *Client) GetUsers(ctx context.Context) ([]backlog.User, error) {
	return c.backlogClient.GetUsers(ctx)
}

// GetPriorities は優先度一覧を取得する
func (c *Client) GetPriorities(ctx context.Context) ([]backlog.Priority, error) {
	return c.backlogClient.GetPriorities(ctx)
}

// GetResolutions は解決状況一覧を取得する
func (c *Client) GetResolutions(ctx context.Context) ([]backlog.Resolution, error) {
	return c.backlogClient.GetResolutions(ctx)
}

// GetCustomFields はカスタムフィールド一覧を取得する
func (c *Client) GetCustomFields(ctx context.Context, projectIDOrKey string) ([]backlog.CustomField, error) {
	return c.backlogClient.GetCustomFields(ctx, backlog.GetCustomFieldsParams{
		ProjectIdOrKey: projectIDOrKey,
	})
}

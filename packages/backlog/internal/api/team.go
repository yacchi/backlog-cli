package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// Team はチーム情報（スペースレベルのユーザーグループ）
type Team struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Members      []User `json:"members"`
	DisplayOrder int    `json:"displayOrder"`
	CreatedUser  *User  `json:"createdUser"`
	Created      string `json:"created"`
	UpdatedUser  *User  `json:"updatedUser"`
	Updated      string `json:"updated"`
}

// TeamListOptions はスペース全体のチーム一覧取得オプション
type TeamListOptions struct {
	Order  string // "asc" or "desc"
	Offset int
	Count  int
}

// ToQuery はクエリパラメータに変換する
func (o *TeamListOptions) ToQuery() url.Values {
	q := url.Values{}
	if o.Order != "" {
		q.Set("order", o.Order)
	}
	if o.Offset > 0 {
		q.Set("offset", strconv.Itoa(o.Offset))
	}
	if o.Count > 0 {
		q.Set("count", strconv.Itoa(o.Count))
	}
	return q
}

// GetTeams はスペース全体のチーム一覧を取得する
func (c *Client) GetTeams(ctx context.Context, opts *TeamListOptions) ([]Team, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}

	resp, err := c.Get(ctx, "/teams", query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var teams []Team
	if err := DecodeResponse(resp, &teams); err != nil {
		return nil, err
	}

	return teams, nil
}

// GetTeam は指定したチームの情報を取得する
func (c *Client) GetTeam(ctx context.Context, teamID int) (*Team, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/teams/%d", teamID), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var team Team
	if err := DecodeResponse(resp, &team); err != nil {
		return nil, err
	}

	return &team, nil
}

// GetProjectTeams は指定したプロジェクトに割り当てられたチームの一覧を取得する
func (c *Client) GetProjectTeams(ctx context.Context, projectIDOrKey string) ([]Team, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/projects/%s/teams", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var teams []Team
	if err := DecodeResponse(resp, &teams); err != nil {
		return nil, err
	}

	return teams, nil
}

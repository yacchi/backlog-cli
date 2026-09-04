package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// Star はスター情報。型定義は issue.go にある（GetIssue のレスポンスに
// 含まれる stars フィールドと共通の構造）。

// AddStarInput はスター追加の入力。
// IssueID / CommentID / WikiID / PullRequestID のうちいずれか1つのみを指定する。
type AddStarInput struct {
	IssueID       int
	CommentID     int
	WikiID        int
	PullRequestID int
}

// AddStar は課題・コメント・Wikiページ・プルリクエストにスターを1つ追加する。
// レスポンスボディは無い(204 No Content)。
func (c *Client) AddStar(ctx context.Context, input *AddStarInput) error {
	data := url.Values{}
	if input.IssueID != 0 {
		data.Set("issueId", strconv.Itoa(input.IssueID))
	}
	if input.CommentID != 0 {
		data.Set("commentId", strconv.Itoa(input.CommentID))
	}
	if input.WikiID != 0 {
		data.Set("wikiId", strconv.Itoa(input.WikiID))
	}
	if input.PullRequestID != 0 {
		data.Set("pullRequestId", strconv.Itoa(input.PullRequestID))
	}

	resp, err := c.PostForm(ctx, "/stars", data)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	return DecodeResponse(resp, nil)
}

// DeleteStar はスターを1つ削除する。starID は対象アイテムのIDではなく
// スター自体のID（backlog star list で取得できるもの）を指定する。
// レスポンスボディは無い(204 No Content)。
func (c *Client) DeleteStar(ctx context.Context, starID int) error {
	resp, err := c.Delete(ctx, fmt.Sprintf("/stars/%d", starID))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	return DecodeResponse(resp, nil)
}

// StarListOptions はスター一覧取得オプション
type StarListOptions struct {
	MinID int
	MaxID int
	Count int
	Order string // "asc" or "desc"
}

// ToQuery はクエリパラメータに変換する
func (o *StarListOptions) ToQuery() url.Values {
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

// GetStars は指定ユーザーが受け取ったスターの一覧を取得する
func (c *Client) GetStars(ctx context.Context, userID int, opts *StarListOptions) ([]Star, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}

	resp, err := c.Get(ctx, fmt.Sprintf("/users/%d/stars", userID), query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var stars []Star
	if err := DecodeResponse(resp, &stars); err != nil {
		return nil, err
	}

	return stars, nil
}

// GetStarsCount は指定ユーザーが受け取ったスターの数を取得する
func (c *Client) GetStarsCount(ctx context.Context, userID int) (int, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/users/%d/stars/count", userID), nil)
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

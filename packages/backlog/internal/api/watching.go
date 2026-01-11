package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// WatchingIssue はウォッチ対象の課題
type WatchingIssue struct {
	ID       int    `json:"id"`
	IssueKey string `json:"issueKey"`
	Summary  string `json:"summary"`
}

// Watching はウォッチ情報
type Watching struct {
	ID                  int           `json:"id"`
	ResourceAlreadyRead bool          `json:"resourceAlreadyRead"`
	Note                string        `json:"note"`
	Type                string        `json:"type"`
	Issue               WatchingIssue `json:"issue"`
	LastContentUpdated  string        `json:"lastContentUpdated"`
	Created             string        `json:"created"`
	Updated             string        `json:"updated"`
}

// WatchingListOptions はウォッチ一覧取得オプション
type WatchingListOptions struct {
	Order               string // "asc" or "desc"
	Sort                string // "created", "updated", "issueUpdated"
	Count               int
	Offset              int
	ResourceAlreadyRead *bool
	IssueIDs            []int
}

// ToQuery はクエリパラメータに変換する
func (o *WatchingListOptions) ToQuery() url.Values {
	q := url.Values{}
	if o.Order != "" {
		q.Set("order", o.Order)
	}
	if o.Sort != "" {
		q.Set("sort", o.Sort)
	}
	if o.Count > 0 {
		q.Set("count", strconv.Itoa(o.Count))
	}
	if o.Offset > 0 {
		q.Set("offset", strconv.Itoa(o.Offset))
	}
	if o.ResourceAlreadyRead != nil {
		q.Set("resourceAlreadyRead", strconv.FormatBool(*o.ResourceAlreadyRead))
	}
	for _, id := range o.IssueIDs {
		q.Add("issueId[]", strconv.Itoa(id))
	}
	return q
}

// GetWatchingList はウォッチ一覧を取得する
func (c *Client) GetWatchingList(ctx context.Context, userID int, opts *WatchingListOptions) ([]Watching, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}

	resp, err := c.Get(ctx, fmt.Sprintf("/users/%d/watchings", userID), query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var watchings []Watching
	if err := DecodeResponse(resp, &watchings); err != nil {
		return nil, err
	}

	return watchings, nil
}

// GetWatchingCount はウォッチ数を取得する
func (c *Client) GetWatchingCount(ctx context.Context, userID int, opts *WatchingListOptions) (int, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}

	resp, err := c.Get(ctx, fmt.Sprintf("/users/%d/watchings/count", userID), query)
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

// AddWatchingInput はウォッチ追加の入力
type AddWatchingInput struct {
	IssueIDOrKey string
	Note         string
}

// AddWatching はウォッチを追加する
func (c *Client) AddWatching(ctx context.Context, input *AddWatchingInput) (*Watching, error) {
	data := url.Values{}
	data.Set("issueIdOrKey", input.IssueIDOrKey)
	if input.Note != "" {
		data.Set("note", input.Note)
	}

	resp, err := c.PostForm(ctx, "/watchings", data)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var watching Watching
	if err := DecodeResponse(resp, &watching); err != nil {
		return nil, err
	}

	return &watching, nil
}

// DeleteWatching はウォッチを削除する
func (c *Client) DeleteWatching(ctx context.Context, watchingID int) (*Watching, error) {
	resp, err := c.Delete(ctx, fmt.Sprintf("/watchings/%d", watchingID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var watching Watching
	if err := DecodeResponse(resp, &watching); err != nil {
		return nil, err
	}

	return &watching, nil
}

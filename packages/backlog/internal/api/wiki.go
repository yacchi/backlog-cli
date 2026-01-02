package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// Wiki はWikiページ
type Wiki struct {
	ID          int          `json:"id"`
	ProjectID   int          `json:"projectId"`
	Name        string       `json:"name"`
	Content     string       `json:"content"`
	Tags        []WikiTag    `json:"tags"`
	Attachments []Attachment `json:"attachments"`
	SharedFiles []SharedFile `json:"sharedFiles"`
	Stars       []Star       `json:"stars"`
	CreatedUser User         `json:"createdUser"`
	Created     string       `json:"created"`
	UpdatedUser *User        `json:"updatedUser"`
	Updated     string       `json:"updated"`
}

// WikiTag はWikiタグ
type WikiTag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// GetWikis はWiki一覧を取得する
func (c *Client) GetWikis(ctx context.Context, projectIDOrKey string) ([]Wiki, error) {
	query := url.Values{}
	query.Set("projectIdOrKey", projectIDOrKey)

	resp, err := c.Get(ctx, "/wikis", query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var wikis []Wiki
	if err := DecodeResponse(resp, &wikis); err != nil {
		return nil, err
	}

	return wikis, nil
}

// GetWiki はWikiページを取得する
func (c *Client) GetWiki(ctx context.Context, wikiID int) (*Wiki, error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/wikis/%d", wikiID), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var wiki Wiki
	if err := DecodeResponse(resp, &wiki); err != nil {
		return nil, err
	}

	return &wiki, nil
}

// GetWikisCount はWikiページ数を取得する
func (c *Client) GetWikisCount(ctx context.Context, projectIDOrKey string) (int, error) {
	query := url.Values{}
	query.Set("projectIdOrKey", projectIDOrKey)

	resp, err := c.Get(ctx, "/wikis/count", query)
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

// CreateWikiInput はWiki作成の入力
type CreateWikiInput struct {
	ProjectID  int
	Name       string
	Content    string
	MailNotify bool
}

// CreateWiki はWikiページを作成する
func (c *Client) CreateWiki(ctx context.Context, input *CreateWikiInput) (*Wiki, error) {
	data := url.Values{}
	data.Set("projectId", strconv.Itoa(input.ProjectID))
	data.Set("name", input.Name)
	data.Set("content", input.Content)
	if input.MailNotify {
		data.Set("mailNotify", "true")
	}

	resp, err := c.PostForm(ctx, "/wikis", data)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var wiki Wiki
	if err := DecodeResponse(resp, &wiki); err != nil {
		return nil, err
	}

	return &wiki, nil
}

// UpdateWikiInput はWiki更新の入力
type UpdateWikiInput struct {
	Name       *string
	Content    *string
	MailNotify bool
}

// UpdateWiki はWikiページを更新する
func (c *Client) UpdateWiki(ctx context.Context, wikiID int, input *UpdateWikiInput) (*Wiki, error) {
	data := url.Values{}
	if input.Name != nil {
		data.Set("name", *input.Name)
	}
	if input.Content != nil {
		data.Set("content", *input.Content)
	}
	if input.MailNotify {
		data.Set("mailNotify", "true")
	}

	resp, err := c.PatchForm(ctx, fmt.Sprintf("/wikis/%d", wikiID), data)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var wiki Wiki
	if err := DecodeResponse(resp, &wiki); err != nil {
		return nil, err
	}

	return &wiki, nil
}

// DeleteWiki はWikiページを削除する
func (c *Client) DeleteWiki(ctx context.Context, wikiID int) (*Wiki, error) {
	resp, err := c.Delete(ctx, fmt.Sprintf("/wikis/%d", wikiID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var wiki Wiki
	if err := DecodeResponse(resp, &wiki); err != nil {
		return nil, err
	}

	return &wiki, nil
}

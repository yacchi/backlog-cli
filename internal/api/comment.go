package api

import (
	"fmt"
	"net/url"
	"strconv"
)

// Comment はコメント
type Comment struct {
	ID            int            `json:"id"`
	Content       string         `json:"content"`
	ChangeLog     []ChangeLog    `json:"changeLog"`
	CreatedUser   User           `json:"createdUser"`
	Created       string         `json:"created"`
	Updated       string         `json:"updated"`
	Stars         []Star         `json:"stars"`
	Notifications []Notification `json:"notifications"`
}

// ChangeLog は変更ログ
type ChangeLog struct {
	Field         string `json:"field"`
	NewValue      string `json:"newValue"`
	OriginalValue string `json:"originalValue"`
}

// Notification は通知
type Notification struct {
	ID                  int  `json:"id"`
	AlreadyRead         bool `json:"alreadyRead"`
	Reason              int  `json:"reason"`
	User                User `json:"user"`
	ResourceAlreadyRead bool `json:"resourceAlreadyRead"`
}

// CommentListOptions はコメント一覧取得オプション
type CommentListOptions struct {
	MinID int
	MaxID int
	Count int
	Order string
}

// ToQuery はクエリパラメータに変換する
func (o *CommentListOptions) ToQuery() url.Values {
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

// GetComments は課題のコメント一覧を取得する
func (c *Client) GetComments(issueIDOrKey string, opts *CommentListOptions) ([]Comment, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}

	resp, err := c.Get(fmt.Sprintf("/issues/%s/comments", issueIDOrKey), query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var comments []Comment
	if err := DecodeResponse(resp, &comments); err != nil {
		return nil, err
	}

	return comments, nil
}

// GetCommentsCount は課題のコメント数を取得する
func (c *Client) GetCommentsCount(issueIDOrKey string) (int, error) {
	resp, err := c.Get(fmt.Sprintf("/issues/%s/comments/count", issueIDOrKey), nil)
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

// GetComment は課題のコメントを取得する
func (c *Client) GetComment(issueIDOrKey string, commentID int) (*Comment, error) {
	resp, err := c.Get(fmt.Sprintf("/issues/%s/comments/%d", issueIDOrKey, commentID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var comment Comment
	if err := DecodeResponse(resp, &comment); err != nil {
		return nil, err
	}

	return &comment, nil
}

// AddComment は課題にコメントを追加する
func (c *Client) AddComment(issueIDOrKey string, content string, notifiedUserIDs []int) (*Comment, error) {
	data := url.Values{}
	data.Set("content", content)
	for _, id := range notifiedUserIDs {
		data.Add("notifiedUserId[]", strconv.Itoa(id))
	}

	resp, err := c.PostForm(fmt.Sprintf("/issues/%s/comments", issueIDOrKey), data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var comment Comment
	if err := DecodeResponse(resp, &comment); err != nil {
		return nil, err
	}

	return &comment, nil
}

// UpdateComment は課題のコメントを更新する
func (c *Client) UpdateComment(issueIDOrKey string, commentID int, content string) (*Comment, error) {
	data := url.Values{}
	data.Set("content", content)

	resp, err := c.PatchForm(fmt.Sprintf("/issues/%s/comments/%d", issueIDOrKey, commentID), data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var comment Comment
	if err := DecodeResponse(resp, &comment); err != nil {
		return nil, err
	}

	return &comment, nil
}

package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// NotificationReason は通知理由
type NotificationReason struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// NotificationIssue は通知に関連する課題
type NotificationIssue struct {
	ID       int    `json:"id"`
	IssueKey string `json:"issueKey"`
	Summary  string `json:"summary"`
}

// NotificationComment は通知に関連するコメント
type NotificationComment struct {
	ID      int    `json:"id"`
	Content string `json:"content"`
}

// NotificationPR は通知に関連するPR
type NotificationPR struct {
	ID     int `json:"id"`
	Number int `json:"number"`
}

// UserNotification はユーザー通知
type UserNotification struct {
	ID                  int                  `json:"id"`
	AlreadyRead         bool                 `json:"alreadyRead"`
	Reason              NotificationReason   `json:"reason"`
	ResourceAlreadyRead bool                 `json:"resourceAlreadyRead"`
	Project             Project              `json:"project"`
	Issue               *NotificationIssue   `json:"issue"`
	Comment             *NotificationComment `json:"comment"`
	PullRequest         *NotificationPR      `json:"pullRequest"`
	PullRequestComment  *NotificationComment `json:"pullRequestComment"`
	Sender              User                 `json:"sender"`
	Created             string               `json:"created"`
}

// NotificationListOptions は通知一覧取得オプション
type NotificationListOptions struct {
	MinID           int
	MaxID           int
	Count           int
	Order           string // "asc" or "desc"
	ResourceAlready bool
}

// ToQuery はクエリパラメータに変換する
func (o *NotificationListOptions) ToQuery() url.Values {
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

// GetNotifications は通知一覧を取得する
func (c *Client) GetNotifications(ctx context.Context, opts *NotificationListOptions) ([]UserNotification, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}

	resp, err := c.Get(ctx, "/notifications", query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var notifications []UserNotification
	if err := DecodeResponse(resp, &notifications); err != nil {
		return nil, err
	}

	return notifications, nil
}

// GetNotificationsCount は未読通知数を取得する
func (c *Client) GetNotificationsCount(ctx context.Context) (int, error) {
	resp, err := c.Get(ctx, "/notifications/count", nil)
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

// MarkNotificationAsRead は通知を既読にする
func (c *Client) MarkNotificationAsRead(ctx context.Context, notificationID int) error {
	resp, err := c.PostForm(ctx, fmt.Sprintf("/notifications/%d/markAsRead", notificationID), nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	return nil
}

// ResetUnreadNotificationCount は未読通知カウントをリセットする（全て既読）
func (c *Client) ResetUnreadNotificationCount(ctx context.Context) (int, error) {
	resp, err := c.PostForm(ctx, "/notifications/markAsRead", nil)
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

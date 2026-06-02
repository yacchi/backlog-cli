package api

import (
	"context"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
)

// ActivityListOptions はユーザーアクティビティ取得オプション。
// activities API は日付レンジ引数を持たないため、期間絞り込みは
// 呼び出し側で minId/maxId ページング + created のクライアントフィルタで吸収する。
type ActivityListOptions struct {
	UserID          int
	ActivityTypeIDs []int
	MinID           int
	MaxID           int
	Count           int
	Order           string // asc, desc
}

// GetUserActivities はユーザーの最近の活動一覧を取得する
func (c *Client) GetUserActivities(ctx context.Context, opts *ActivityListOptions) ([]backlog.Activity, error) {
	params := backlog.GetUserRecentUpdatesParams{}
	if opts != nil {
		params.UserId = opts.UserID
		params.ActivityTypeId = opts.ActivityTypeIDs
		if opts.MinID > 0 {
			params.MinId = backlog.NewOptInt(opts.MinID)
		}
		if opts.MaxID > 0 {
			params.MaxId = backlog.NewOptInt(opts.MaxID)
		}
		if opts.Count > 0 {
			params.Count = backlog.NewOptInt(opts.Count)
		}
		if opts.Order != "" {
			params.Order = backlog.NewOptString(opts.Order)
		}
	}

	return c.backlogClient.GetUserRecentUpdates(ctx, params)
}

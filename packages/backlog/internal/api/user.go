package api

import (
	"context"
	"fmt"

	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
)

// User はユーザー情報
type User struct {
	ID            int           `json:"id"`
	UserID        string        `json:"userId"`
	Name          string        `json:"name"`
	RoleType      int           `json:"roleType"`
	Lang          string        `json:"lang"`
	MailAddress   string        `json:"mailAddress"`
	NulabAccount  *NulabAccount `json:"nulabAccount"`
	LastLoginTime string        `json:"lastLoginTime"`
}

// NulabAccount は Nulab アカウント情報
type NulabAccount struct {
	NulabID  string `json:"nulabId"`
	Name     string `json:"name"`
	UniqueID string `json:"uniqueId"`
}

// GetCurrentUser は認証ユーザーの情報を取得する
func (c *Client) GetCurrentUser(ctx context.Context) (*backlog.User, error) {
	return c.backlogClient.GetCurrentUser(ctx)
}

// GetUser はユーザー情報を取得する
func (c *Client) GetUser(ctx context.Context, userID int) (*backlog.User, error) {
	return c.backlogClient.GetUser(ctx, backlog.GetUserParams{
		UserId: userID,
	})
}

// RecentlyViewedIssuesOptions は最近見た課題一覧取得オプション
type RecentlyViewedIssuesOptions struct {
	Order  string // asc, desc
	Offset int
	Count  int
}

// GetRecentlyViewedIssues は認証ユーザーが最近見た課題の一覧を取得する
func (c *Client) GetRecentlyViewedIssues(ctx context.Context, opts *RecentlyViewedIssuesOptions) ([]backlog.RecentlyViewedIssue, error) {
	params := backlog.GetListOfRecentlyViewedIssuesParams{}
	if opts != nil {
		if opts.Order != "" {
			params.Order = backlog.NewOptString(opts.Order)
		}
		if opts.Offset > 0 {
			params.Offset = backlog.NewOptInt(opts.Offset)
		}
		if opts.Count > 0 {
			params.Count = backlog.NewOptInt(opts.Count)
		}
	}

	return c.backlogClient.GetListOfRecentlyViewedIssues(ctx, params)
}

// FetchCurrentUser はアクセストークンを使用して認証ユーザー情報を取得する
// Client を使用せずに直接 HTTP リクエストを行うスタンドアロン関数
// 中継サーバーなど、Client を構築できない場面で使用する
func FetchCurrentUser(ctx context.Context, domain, space, accessToken string) (*backlog.User, error) {
	// 簡易的な SecuritySource 実装
	tokenSource := &simpleTokenSource{token: accessToken}

	// URL構築
	baseURL := fmt.Sprintf("https://%s.%s/api/v2", space, domain)

	client, err := backlog.NewClient(baseURL, tokenSource)
	if err != nil {
		return nil, fmt.Errorf("failed to create backlog client: %w", err)
	}

	return client.GetCurrentUser(ctx)
}

type simpleTokenSource struct {
	token string
}

func (s *simpleTokenSource) ApiKey(ctx context.Context, operationName backlog.OperationName) (backlog.ApiKey, error) {
	return backlog.ApiKey{}, ogenerrors.ErrSkipClientSecurity
}

func (s *simpleTokenSource) OAuth2(ctx context.Context, operationName backlog.OperationName) (backlog.OAuth2, error) {
	return backlog.OAuth2{Token: s.token}, nil
}

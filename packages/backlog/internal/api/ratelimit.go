package api

import "context"

// RateLimitCategory はレート制限の1カテゴリ分の情報
type RateLimitCategory struct {
	Limit     int   `json:"limit"`
	Remaining int   `json:"remaining"`
	Reset     int64 `json:"reset"`
}

// RateLimit はカテゴリ別のレート制限情報
// Backlog のレート制限はスペース単位かつカテゴリ（read/update/search/icon）ごとに
// 独立して管理されており、単一のグローバルな上限ではない。
type RateLimit struct {
	Read   RateLimitCategory `json:"read"`
	Update RateLimitCategory `json:"update"`
	Search RateLimitCategory `json:"search"`
	Icon   RateLimitCategory `json:"icon"`
}

// rateLimitResponse は GET /rateLimit の生レスポンス
type rateLimitResponse struct {
	RateLimit RateLimit `json:"rateLimit"`
}

// GetRateLimit は使用中のAPIキー/トークンに対応するユーザーの
// 現在のレート制限情報を取得する
func (c *Client) GetRateLimit(ctx context.Context) (*RateLimit, error) {
	resp, err := c.Get(ctx, "/rateLimit", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result rateLimitResponse
	if err := DecodeResponse(resp, &result); err != nil {
		return nil, err
	}

	return &result.RateLimit, nil
}

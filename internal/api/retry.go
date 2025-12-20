package api

import (
	"net/http"
	"strconv"
	"time"
)

// RetryTransport はレートリミット（429）に対するリトライを行うRoundTripper
type RetryTransport struct {
	Base       http.RoundTripper
	MaxRetries int
}

func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	var resp *http.Response
	var err error

	for i := 0; i <= t.MaxRetries; i++ {
		resp, err = base.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		// 429 Too Many Requests 以外はそのまま返す
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}

		// リトライ上限に達したらそのまま返す
		if i == t.MaxRetries {
			return resp, nil
		}

		// レスポンスボディを閉じておく（リトライするため）
		_ = resp.Body.Close()

		// 待機時間を決定
		waitDuration := t.getWaitDuration(resp)

		// 待機してリトライ
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(waitDuration):
			// リトライ継続
			continue
		}
	}

	return resp, nil
}

func (t *RetryTransport) getWaitDuration(resp *http.Response) time.Duration {
	// Retry-After ヘッダーを確認
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			return time.Duration(seconds) * time.Second
		}
		// 日付形式の場合のパースも可能だが、Backlog APIは秒数を返すのが一般的
		// 必要であれば http.ParseTime などを使用
	}

	// X-RateLimit-Reset ヘッダーを確認 (Unix Timestamp)
	reset := resp.Header.Get("X-RateLimit-Reset")
	if reset != "" {
		if resetTime, err := strconv.ParseInt(reset, 10, 64); err == nil {
			wait := time.Until(time.Unix(resetTime, 0))
			if wait > 0 {
				return wait
			}
		}
	}

	// デフォルト待機時間（ヘッダーがない、またはパースできない場合）
	return 60 * time.Second
}

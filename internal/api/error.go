package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIError は Backlog API エラー
type APIError struct {
	StatusCode int
	Errors     []ErrorDetail `json:"errors"`
}

// ErrorDetail はエラー詳細
type ErrorDetail struct {
	Message  string `json:"message"`
	Code     int    `json:"code"`
	MoreInfo string `json:"moreInfo"`
}

func (e *APIError) Error() string {
	if len(e.Errors) > 0 {
		return fmt.Sprintf("Backlog API error: %s (code: %d)", e.Errors[0].Message, e.Errors[0].Code)
	}
	return fmt.Sprintf("Backlog API error: status %d", e.StatusCode)
}

// CheckResponse はレスポンスをチェックし、エラーがあれば返す
func CheckResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	apiErr := &APIError{StatusCode: resp.StatusCode}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return apiErr
	}

	// Backlog API のエラー形式をパース
	var errResp struct {
		Errors []ErrorDetail `json:"errors"`
	}
	if json.Unmarshal(body, &errResp) == nil {
		apiErr.Errors = errResp.Errors
	}

	return apiErr
}

// DecodeResponse はレスポンスをデコードする
func DecodeResponse(resp *http.Response, v interface{}) error {
	if err := CheckResponse(resp); err != nil {
		return err
	}

	if v == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(v)
}

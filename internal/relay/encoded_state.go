package relay

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// EncodedStateClaims はエンコードされた state の構造
type EncodedStateClaims struct {
	Port     int    `json:"p"`          // CLI のローカルサーバーポート
	CLIState string `json:"s"`          // CLI が生成した state
	Space    string `json:"sp"`         // Backlog スペース名
	Domain   string `json:"d"`          // Backlog ドメイン
	Project  string `json:"pr,omitempty"` // プロジェクト（オプション）
}

// encodeState は state をエンコードする
// フォーマット: base64url(JSON)
func encodeState(claims EncodedStateClaims) (string, error) {
	data, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal claims: %w", err)
	}
	return base64.URLEncoding.EncodeToString(data), nil
}

// decodeState はエンコードされた state をデコードする
func decodeState(encoded string) (*EncodedStateClaims, error) {
	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	var claims EncodedStateClaims
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, fmt.Errorf("failed to unmarshal claims: %w", err)
	}

	// 必須フィールドの検証
	if claims.Port == 0 || claims.CLIState == "" || claims.Space == "" || claims.Domain == "" {
		return nil, fmt.Errorf("invalid claims: missing required fields")
	}

	return &claims, nil
}

package relay

import (
	"net"
	"net/http"
	"strings"
)

// RequestContext はHTTPリクエストから抽出した情報を保持する
// プロキシ経由のリクエストを考慮し、オリジナルの情報を取得する
type RequestContext struct {
	// Scheme はリクエストのスキーム（http/https）
	Scheme string

	// Host はオリジナルのホスト名
	Host string

	// BaseURL はスキームとホストから構築したベースURL
	BaseURL string

	// ClientIP はクライアントのIPアドレス
	ClientIP string

	// UserAgent はUser-Agentヘッダーの値
	UserAgent string
}

// ExtractRequestContext はHTTPリクエストから情報を抽出する
//
// 優先順位:
//   - Scheme: X-Forwarded-Proto > "https"（デフォルト）
//   - Host: X-Original-Host > X-Forwarded-Host > Host ヘッダー
//   - ClientIP: X-Forwarded-For > X-Real-IP > RemoteAddr
func ExtractRequestContext(r *http.Request) RequestContext {
	if r == nil {
		return RequestContext{Scheme: "https"}
	}

	scheme := extractScheme(r)
	host := extractHost(r)
	baseURL := ""
	if host != "" {
		baseURL = scheme + "://" + host
	}

	return RequestContext{
		Scheme:    scheme,
		Host:      host,
		BaseURL:   baseURL,
		ClientIP:  extractClientIPString(r),
		UserAgent: r.UserAgent(),
	}
}

// extractScheme はリクエストからスキームを抽出する
func extractScheme(r *http.Request) string {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	return "https"
}

// extractHost はリクエストからオリジナルホストを抽出する
func extractHost(r *http.Request) string {
	if origHost := r.Header.Get("X-Original-Host"); origHost != "" {
		return origHost
	}
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		return fwdHost
	}
	return r.Host
}

// extractClientIP はリクエストからクライアントIPを抽出する
// X-Forwarded-For > X-Real-IP > RemoteAddr の優先順位
func extractClientIP(r *http.Request) net.IP {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			if ip := net.ParseIP(strings.TrimSpace(ips[0])); ip != nil {
				return ip
			}
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := net.ParseIP(xri); ip != nil {
			return ip
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}

// extractClientIPString はリクエストからクライアントIPを文字列で抽出する
func extractClientIPString(r *http.Request) string {
	if ip := extractClientIP(r); ip != nil {
		return ip.String()
	}
	return ""
}

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newRedirectingTransport は、baseURL 宛のリクエストを httptest サーバーへ実際に転送する
// RoundTripper を返す。ヘッダー・クエリの実送信内容を httptest サーバー側で検証できるようにする。
func newRedirectingTransport(t *testing.T, target *url.URL) http.RoundTripper {
	t.Helper()
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(req)
	})
}

// TestAPIKeySentAsHeaderNotQuery は API Key 認証時に、apiKey がクエリ文字列ではなく
// Backlog-API-Key ヘッダーで送信されることを確認する。
func TestAPIKeySentAsHeaderNotQuery(t *testing.T) {
	type captured struct {
		headerValue string
		hasHeader   bool
		rawQuery    string
	}

	tests := []struct {
		name string
		call func(t *testing.T, client *Client) captured
	}{
		{
			name: "Get",
			call: func(t *testing.T, client *Client) captured {
				var got captured
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					got.headerValue = r.Header.Get("Backlog-API-Key")
					_, got.hasHeader = r.Header["Backlog-Api-Key"]
					got.rawQuery = r.URL.RawQuery
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				target, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				client.httpClient.Transport = newRedirectingTransport(t, target)

				resp, err := client.Get(context.Background(), "/issues", url.Values{"count": {"1"}})
				if err != nil {
					t.Fatalf("Get returned error: %v", err)
				}
				_ = resp.Body.Close()
				return got
			},
		},
		{
			name: "PostForm",
			call: func(t *testing.T, client *Client) captured {
				var got captured
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					got.headerValue = r.Header.Get("Backlog-API-Key")
					_, got.hasHeader = r.Header["Backlog-Api-Key"]
					got.rawQuery = r.URL.RawQuery
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				target, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				client.httpClient.Transport = newRedirectingTransport(t, target)

				resp, err := client.PostForm(context.Background(), "/issues/1/comments", url.Values{"content": {"hi"}})
				if err != nil {
					t.Fatalf("PostForm returned error: %v", err)
				}
				_ = resp.Body.Close()
				return got
			},
		},
		{
			name: "Delete",
			call: func(t *testing.T, client *Client) captured {
				var got captured
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					got.headerValue = r.Header.Get("Backlog-API-Key")
					_, got.hasHeader = r.Header["Backlog-Api-Key"]
					got.rawQuery = r.URL.RawQuery
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				target, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				client.httpClient.Transport = newRedirectingTransport(t, target)

				resp, err := client.Delete(context.Background(), "/issues/1")
				if err != nil {
					t.Fatalf("Delete returned error: %v", err)
				}
				_ = resp.Body.Close()
				return got
			},
		},
		{
			name: "RawRequest",
			call: func(t *testing.T, client *Client) captured {
				var got captured
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					got.headerValue = r.Header.Get("Backlog-API-Key")
					_, got.hasHeader = r.Header["Backlog-Api-Key"]
					got.rawQuery = r.URL.RawQuery
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				target, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				client.httpClient.Transport = newRedirectingTransport(t, target)

				resp, err := client.RawRequest(context.Background(), http.MethodGet, "/api/v2/space", nil, nil, "")
				if err != nil {
					t.Fatalf("RawRequest returned error: %v", err)
				}
				_ = resp.Body.Close()
				return got
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient("example.backlog.jp", "", WithAPIKey("k"))
			got := tt.call(t, client)

			if !got.hasHeader {
				t.Fatalf("expected Backlog-API-Key header to be sent")
			}
			if got.headerValue != "k" {
				t.Fatalf("Backlog-API-Key header = %q, want %q", got.headerValue, "k")
			}

			query, err := url.ParseQuery(got.rawQuery)
			if err != nil {
				t.Fatalf("failed to parse RawQuery %q: %v", got.rawQuery, err)
			}
			if _, ok := query["apiKey"]; ok {
				t.Fatalf("RawQuery %q unexpectedly contains apiKey", got.rawQuery)
			}
		})
	}
}

// TestOAuthSendsBearerAuthorizationNoAPIKeyHeader は OAuth アクセストークンで構築した
// クライアントが Authorization: Bearer ヘッダーを送信し、Backlog-API-Key ヘッダーを
// 送信しないことを確認する。
func TestOAuthSendsBearerAuthorizationNoAPIKeyHeader(t *testing.T) {
	var (
		authHeader     string
		hasAPIKeyHdr   bool
		requestReached bool
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReached = true
		authHeader = r.Header.Get("Authorization")
		_, hasAPIKeyHdr = r.Header["Backlog-Api-Key"]
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, strings.NewReader(""))
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	client := NewClient("example.backlog.jp", "test-access-token")
	client.httpClient.Transport = newRedirectingTransport(t, target)

	resp, err := client.Get(context.Background(), "/issues", nil)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	_ = resp.Body.Close()

	if !requestReached {
		t.Fatal("request never reached the test server")
	}
	if authHeader != "Bearer test-access-token" {
		t.Fatalf("Authorization header = %q, want %q", authHeader, "Bearer test-access-token")
	}
	if hasAPIKeyHdr {
		t.Fatal("Backlog-API-Key header should not be sent for OAuth clients")
	}
}

// TestAPIKeySentAsHeaderNotQuery_PatchFormAndDeleteWithForm は、契約で明示された
// Get/PostForm/Delete/RawRequest の4系統に加えて client.go 内の他の apiKey 送出箇所
// (PatchForm, DeleteWithForm) も同様にヘッダー送信へ切り替わっており、クエリに
// apiKey が一切残っていないことを確認する。A3 は「APIキーがURL/クエリに置かれる
// 箇所は全て」の書き換えを要求しており、PatchForm/DeleteWithForm もその対象である。
func TestAPIKeySentAsHeaderNotQuery_PatchFormAndDeleteWithForm(t *testing.T) {
	tests := []struct {
		name string
		call func(t *testing.T, client *Client) (headerValue string, hasHeader bool, rawQuery string)
	}{
		{
			name: "PatchForm",
			call: func(t *testing.T, client *Client) (string, bool, string) {
				var headerValue, rawQuery string
				var hasHeader bool
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					headerValue = r.Header.Get("Backlog-API-Key")
					_, hasHeader = r.Header["Backlog-Api-Key"]
					rawQuery = r.URL.RawQuery
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				target, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				client.httpClient.Transport = newRedirectingTransport(t, target)

				resp, err := client.PatchForm(context.Background(), "/issues/1", url.Values{"summary": {"hi"}})
				if err != nil {
					t.Fatalf("PatchForm returned error: %v", err)
				}
				_ = resp.Body.Close()
				return headerValue, hasHeader, rawQuery
			},
		},
		{
			name: "DeleteWithForm",
			call: func(t *testing.T, client *Client) (string, bool, string) {
				var headerValue, rawQuery string
				var hasHeader bool
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					headerValue = r.Header.Get("Backlog-API-Key")
					_, hasHeader = r.Header["Backlog-Api-Key"]
					rawQuery = r.URL.RawQuery
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				target, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				client.httpClient.Transport = newRedirectingTransport(t, target)

				resp, err := client.DeleteWithForm(context.Background(), "/issues/1", url.Values{"reason": {"x"}})
				if err != nil {
					t.Fatalf("DeleteWithForm returned error: %v", err)
				}
				_ = resp.Body.Close()
				return headerValue, hasHeader, rawQuery
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient("example.backlog.jp", "", WithAPIKey("k"))
			headerValue, hasHeader, rawQuery := tt.call(t, client)

			if !hasHeader {
				t.Fatalf("expected Backlog-API-Key header to be sent")
			}
			if headerValue != "k" {
				t.Fatalf("Backlog-API-Key header = %q, want %q", headerValue, "k")
			}
			query, err := url.ParseQuery(rawQuery)
			if err != nil {
				t.Fatalf("failed to parse RawQuery %q: %v", rawQuery, err)
			}
			if _, ok := query["apiKey"]; ok {
				t.Fatalf("RawQuery %q unexpectedly contains apiKey", rawQuery)
			}
		})
	}
}

// TestNoAuthClientSendsNoAuthHeaders は、APIキーもOAuthトークンも設定されていない
// クライアントが Backlog-API-Key も Authorization も一切送信しないことを確認する
// （省略可能な認証情報が欠落している場合にフィールドを出さない、という境界ケース）。
func TestNoAuthClientSendsNoAuthHeaders(t *testing.T) {
	var hasAPIKeyHdr, hasAuthHdr bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasAPIKeyHdr = r.Header["Backlog-Api-Key"]
		_, hasAuthHdr = r.Header["Authorization"]
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	client := NewClient("example.backlog.jp", "")
	client.httpClient.Transport = newRedirectingTransport(t, target)

	resp, err := client.Get(context.Background(), "/issues", nil)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	_ = resp.Body.Close()

	if hasAPIKeyHdr {
		t.Fatal("Backlog-API-Key header should not be sent when no API key is configured")
	}
	if hasAuthHdr {
		t.Fatal("Authorization header should not be sent when no OAuth token is configured")
	}
}

// TestAPIKeyPreferredOverOAuthWhenBothSet は、APIキーとOAuthトークンの両方が
// クライアントに設定されている場合、client.go の実装が
// `if c.apiKey == "" && c.accessToken != ""` という条件で Authorization ヘッダーの
// 送出を判定しているため、APIキーが優先され Authorization ヘッダーは送られないことを
// 確認する（優先順位の回帰を検知する）。
func TestAPIKeyPreferredOverOAuthWhenBothSet(t *testing.T) {
	var hasAPIKeyHdr, hasAuthHdr bool
	var apiKeyValue string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKeyValue = r.Header.Get("Backlog-API-Key")
		_, hasAPIKeyHdr = r.Header["Backlog-Api-Key"]
		_, hasAuthHdr = r.Header["Authorization"]
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	client := NewClient("example.backlog.jp", "test-access-token", WithAPIKey("k"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	resp, err := client.Get(context.Background(), "/issues", nil)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	_ = resp.Body.Close()

	if !hasAPIKeyHdr || apiKeyValue != "k" {
		t.Fatalf("expected Backlog-API-Key header %q, hasHeader=%v value=%q", "k", hasAPIKeyHdr, apiKeyValue)
	}
	if hasAuthHdr {
		t.Fatal("Authorization header should not be sent when an API key is also configured")
	}
}

// TestAdversarial_PostAndPatchAlsoUseHeaderNotQuery は、契約 A5 が明示した
// Get/PostForm/Delete/RawRequest の4系統に加え、同じく Request() を経由する
// Post/Patch も Backlog-API-Key ヘッダーで送信され、クエリに apiKey が
// 一切残らないことを確認する（既存テストは Post/Patch を対象にしていない）。
func TestAdversarial_PostAndPatchAlsoUseHeaderNotQuery(t *testing.T) {
	tests := []struct {
		name string
		call func(t *testing.T, client *Client) (headerValue string, hasHeader bool, rawQuery string)
	}{
		{
			name: "Post",
			call: func(t *testing.T, client *Client) (string, bool, string) {
				var headerValue, rawQuery string
				var hasHeader bool
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					headerValue = r.Header.Get("Backlog-API-Key")
					_, hasHeader = r.Header["Backlog-Api-Key"]
					rawQuery = r.URL.RawQuery
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				target, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				client.httpClient.Transport = newRedirectingTransport(t, target)

				resp, err := client.Post(context.Background(), "/issues", map[string]string{"summary": "hi"})
				if err != nil {
					t.Fatalf("Post returned error: %v", err)
				}
				_ = resp.Body.Close()
				return headerValue, hasHeader, rawQuery
			},
		},
		{
			name: "Patch",
			call: func(t *testing.T, client *Client) (string, bool, string) {
				var headerValue, rawQuery string
				var hasHeader bool
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					headerValue = r.Header.Get("Backlog-API-Key")
					_, hasHeader = r.Header["Backlog-Api-Key"]
					rawQuery = r.URL.RawQuery
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				target, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				client.httpClient.Transport = newRedirectingTransport(t, target)

				resp, err := client.Patch(context.Background(), "/issues/1", map[string]string{"summary": "hi"})
				if err != nil {
					t.Fatalf("Patch returned error: %v", err)
				}
				_ = resp.Body.Close()
				return headerValue, hasHeader, rawQuery
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient("example.backlog.jp", "", WithAPIKey("k"))
			headerValue, hasHeader, rawQuery := tt.call(t, client)

			if !hasHeader {
				t.Fatalf("expected Backlog-API-Key header to be sent")
			}
			if headerValue != "k" {
				t.Fatalf("Backlog-API-Key header = %q, want %q", headerValue, "k")
			}
			query, err := url.ParseQuery(rawQuery)
			if err != nil {
				t.Fatalf("failed to parse RawQuery %q: %v", rawQuery, err)
			}
			if _, ok := query["apiKey"]; ok {
				t.Fatalf("RawQuery %q unexpectedly contains apiKey", rawQuery)
			}
		})
	}
}

// TestAdversarial_PathWithExistingQueryString は、呼び出し元が既にクエリ文字列を
// 含むパスを渡した場合でも、apiKey がクエリに紛れ込まず Backlog-API-Key ヘッダーで
// 送信されることを確認する（契約はクエリへの apiKey 混入を一切許さないため、
// パスに既存クエリがあるケースも回帰対象とする）。
func TestAdversarial_PathWithExistingQueryString(t *testing.T) {
	var headerValue, rawQuery string
	var hasHeader bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerValue = r.Header.Get("Backlog-API-Key")
		_, hasHeader = r.Header["Backlog-Api-Key"]
		rawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	client := NewClient("example.backlog.jp", "", WithAPIKey("k"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	// path 自体に既にクエリ文字列が含まれているケース
	resp, err := client.Get(context.Background(), "/issues?already=1", url.Values{"count": {"1"}})
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	_ = resp.Body.Close()

	if !hasHeader || headerValue != "k" {
		t.Fatalf("expected Backlog-API-Key header %q, hasHeader=%v value=%q", "k", hasHeader, headerValue)
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		t.Fatalf("failed to parse RawQuery %q: %v", rawQuery, err)
	}
	if _, ok := query["apiKey"]; ok {
		t.Fatalf("RawQuery %q unexpectedly contains apiKey", rawQuery)
	}
}

// TestAdversarial_BothAPIKeyAndAccessTokenSet_AllHelpers は、APIキーと
// OAuthアクセストークンの両方が設定された状態で、Request() 経由の全ヘルパー
// (Get/Post/Patch/Delete) について APIキーが優先され、Backlog-API-Key
// ヘッダーのみが送られ Authorization ヘッダーは送られないことを確認する。
// 既存の TestAPIKeyPreferredOverOAuthWhenBothSet は Get のみを対象としており、
// 他のヘルパーでの回帰は検知できない。
func TestAdversarial_BothAPIKeyAndAccessTokenSet_AllHelpers(t *testing.T) {
	tests := []struct {
		name string
		call func(t *testing.T, client *Client) (hasAPIKeyHdr, hasAuthHdr bool, apiKeyValue string)
	}{
		{
			name: "Get",
			call: func(t *testing.T, client *Client) (bool, bool, string) {
				var hasAPIKeyHdr, hasAuthHdr bool
				var apiKeyValue string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					apiKeyValue = r.Header.Get("Backlog-API-Key")
					_, hasAPIKeyHdr = r.Header["Backlog-Api-Key"]
					_, hasAuthHdr = r.Header["Authorization"]
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				target, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				client.httpClient.Transport = newRedirectingTransport(t, target)
				resp, err := client.Get(context.Background(), "/issues", nil)
				if err != nil {
					t.Fatalf("Get returned error: %v", err)
				}
				_ = resp.Body.Close()
				return hasAPIKeyHdr, hasAuthHdr, apiKeyValue
			},
		},
		{
			name: "PostForm",
			call: func(t *testing.T, client *Client) (bool, bool, string) {
				var hasAPIKeyHdr, hasAuthHdr bool
				var apiKeyValue string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					apiKeyValue = r.Header.Get("Backlog-API-Key")
					_, hasAPIKeyHdr = r.Header["Backlog-Api-Key"]
					_, hasAuthHdr = r.Header["Authorization"]
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				target, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				client.httpClient.Transport = newRedirectingTransport(t, target)
				resp, err := client.PostForm(context.Background(), "/issues/1/comments", url.Values{"content": {"hi"}})
				if err != nil {
					t.Fatalf("PostForm returned error: %v", err)
				}
				_ = resp.Body.Close()
				return hasAPIKeyHdr, hasAuthHdr, apiKeyValue
			},
		},
		{
			name: "Delete",
			call: func(t *testing.T, client *Client) (bool, bool, string) {
				var hasAPIKeyHdr, hasAuthHdr bool
				var apiKeyValue string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					apiKeyValue = r.Header.Get("Backlog-API-Key")
					_, hasAPIKeyHdr = r.Header["Backlog-Api-Key"]
					_, hasAuthHdr = r.Header["Authorization"]
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				target, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				client.httpClient.Transport = newRedirectingTransport(t, target)
				resp, err := client.Delete(context.Background(), "/issues/1")
				if err != nil {
					t.Fatalf("Delete returned error: %v", err)
				}
				_ = resp.Body.Close()
				return hasAPIKeyHdr, hasAuthHdr, apiKeyValue
			},
		},
		{
			name: "RawRequest",
			call: func(t *testing.T, client *Client) (bool, bool, string) {
				var hasAPIKeyHdr, hasAuthHdr bool
				var apiKeyValue string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					apiKeyValue = r.Header.Get("Backlog-API-Key")
					_, hasAPIKeyHdr = r.Header["Backlog-Api-Key"]
					_, hasAuthHdr = r.Header["Authorization"]
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				target, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				client.httpClient.Transport = newRedirectingTransport(t, target)
				resp, err := client.RawRequest(context.Background(), http.MethodGet, "/api/v2/space", nil, nil, "")
				if err != nil {
					t.Fatalf("RawRequest returned error: %v", err)
				}
				_ = resp.Body.Close()
				return hasAPIKeyHdr, hasAuthHdr, apiKeyValue
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient("example.backlog.jp", "test-access-token", WithAPIKey("k"))
			hasAPIKeyHdr, hasAuthHdr, apiKeyValue := tt.call(t, client)

			if !hasAPIKeyHdr || apiKeyValue != "k" {
				t.Fatalf("expected Backlog-API-Key header %q, hasHeader=%v value=%q", "k", hasAPIKeyHdr, apiKeyValue)
			}
			if hasAuthHdr {
				t.Fatal("Authorization header should not be sent when an API key is also configured")
			}
		})
	}
}

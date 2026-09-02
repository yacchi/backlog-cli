package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGetRateLimitDecodesCategories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/rateLimit") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"rateLimit": {
				"read":   {"limit": 600, "remaining": 599, "reset": 1500000000},
				"update": {"limit": 150, "remaining": 0,   "reset": 1500000100},
				"search": {"limit": 150, "remaining": 149, "reset": 1500000200},
				"icon":   {"limit": 150, "remaining": 149, "reset": 1500000300}
			}
		}`))
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	rl, err := client.GetRateLimit(context.Background())
	if err != nil {
		t.Fatalf("GetRateLimit returned error: %v", err)
	}

	if rl.Read.Limit != 600 || rl.Read.Remaining != 599 || rl.Read.Reset != 1500000000 {
		t.Fatalf("unexpected read category: %+v", rl.Read)
	}
	if rl.Update.Remaining != 0 {
		t.Fatalf("update.remaining = %d, want 0", rl.Update.Remaining)
	}
	if rl.Search.Reset != 1500000200 {
		t.Fatalf("search.reset = %d, want 1500000200", rl.Search.Reset)
	}
	if rl.Icon.Limit != 150 {
		t.Fatalf("icon.limit = %d, want 150", rl.Icon.Limit)
	}
}

func TestGetRateLimitErrorPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"errors":[{"message":"rate limit exceeded","code":9,"moreInfo":""}]}`))
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	_, err = client.GetRateLimit(context.Background())
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusTooManyRequests)
	}
}

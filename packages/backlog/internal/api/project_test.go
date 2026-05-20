package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCreateCategoryAcceptsStatusOK(t *testing.T) {
	var body string

	client := NewClient("example", "backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s, want %s", req.Method, http.MethodPost)
		}
		if req.URL.Path != "/api/v2/projects/PROJ/categories" {
			t.Fatalf("path = %s, want %s", req.URL.Path, "/api/v2/projects/PROJ/categories")
		}

		data, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		body = string(data)

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":123,"name":"Bug","displayOrder":1}`)),
		}, nil
	})

	category, err := client.CreateCategory(context.Background(), "PROJ", "Bug")
	if err != nil {
		t.Fatalf("CreateCategory returned error: %v", err)
	}

	form, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if got := form.Get("name"); got != "Bug" {
		t.Fatalf("name = %q, want %q", got, "Bug")
	}

	if !category.ID.IsSet() || category.ID.Value != 123 {
		t.Fatalf("category.ID = %+v, want 123", category.ID)
	}
	if !category.Name.IsSet() || category.Name.Value != "Bug" {
		t.Fatalf("category.Name = %+v, want %q", category.Name, "Bug")
	}
}

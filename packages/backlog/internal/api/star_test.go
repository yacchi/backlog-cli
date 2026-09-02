package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAddStarSendsExactlyOneTarget(t *testing.T) {
	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/stars") || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		capturedBody = r.PostForm.Encode()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	if err := client.AddStar(context.Background(), &AddStarInput{IssueID: 42}); err != nil {
		t.Fatalf("AddStar returned error: %v", err)
	}

	q, err := url.ParseQuery(capturedBody)
	if err != nil {
		t.Fatal(err)
	}
	if got := q.Get("issueId"); got != "42" {
		t.Fatalf("issueId = %q, want 42", got)
	}
	if q.Has("commentId") || q.Has("wikiId") || q.Has("pullRequestId") {
		t.Fatalf("unexpected extra target fields in body: %s", capturedBody)
	}
}

func TestAddStarErrorPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"message":"invalid target","code":6,"moreInfo":""}]}`))
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	err = client.AddStar(context.Background(), &AddStarInput{IssueID: 1})
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestDeleteStarUsesStarIDPath(t *testing.T) {
	var capturedPath, capturedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	if err := client.DeleteStar(context.Background(), 99); err != nil {
		t.Fatalf("DeleteStar returned error: %v", err)
	}
	if capturedMethod != http.MethodDelete {
		t.Fatalf("method = %s, want DELETE", capturedMethod)
	}
	if !strings.HasSuffix(capturedPath, "/stars/99") {
		t.Fatalf("path = %s, want suffix /stars/99", capturedPath)
	}
}

func TestGetStarsEncodesQuery(t *testing.T) {
	var capturedURL *url.URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":75,"comment":"","url":"https://x.backlog.jp/view/PROJ-1","title":"[PROJ-1] title","presenter":{"id":1,"userId":"admin","name":"Admin","roleType":1,"lang":"ja","mailAddress":"a@example.com"},"created":"2026-01-01T00:00:00Z"}]`))
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	stars, err := client.GetStars(context.Background(), 5, &StarListOptions{MinID: 10, MaxID: 200, Count: 50, Order: "asc"})
	if err != nil {
		t.Fatalf("GetStars returned error: %v", err)
	}

	if !strings.HasSuffix(capturedURL.Path, "/users/5/stars") {
		t.Fatalf("unexpected path: %s", capturedURL.Path)
	}
	q := capturedURL.Query()
	if q.Get("minId") != "10" || q.Get("maxId") != "200" || q.Get("count") != "50" || q.Get("order") != "asc" {
		t.Fatalf("unexpected query: %s", capturedURL.RawQuery)
	}
	if len(stars) != 1 || stars[0].ID != 75 || stars[0].Presenter.UserID != "admin" {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

// TestGetStarsOmitsQueryWhenOptionsZero verifies that leaving minId/maxId/
// count/order at their zero values (the "not specified" state for the
// --min-id/--max-id/--count/--order flags) sends no query string at all,
// letting the API apply its own defaults rather than sending literal 0s.
func TestGetStarsOmitsQueryWhenOptionsZero(t *testing.T) {
	var capturedURL *url.URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	if _, err := client.GetStars(context.Background(), 5, &StarListOptions{}); err != nil {
		t.Fatalf("GetStars returned error: %v", err)
	}

	if capturedURL.RawQuery != "" {
		t.Fatalf("expected no query params for zero-valued options, got: %q", capturedURL.RawQuery)
	}
}

func TestGetStarsCountDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/users/7/stars/count") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"count":3}`))
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	count, err := client.GetStarsCount(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetStarsCount returned error: %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
}

func TestDeleteStarErrorPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"message":"no such star","code":5,"moreInfo":""}]}`))
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	err = client.DeleteStar(context.Background(), 404)
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	// The command layer wraps this with fmt.Errorf("failed to remove star: %w", err) —
	// for that wrap to be useful to a caller (human or agent), the underlying error
	// must carry the API's own message, not just a generic "non-2xx" description.
	if !strings.Contains(err.Error(), "no such star") {
		t.Fatalf("expected error to contain the API's error message, got: %v", err)
	}
}

package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestGetUserActivitiesEncodesQuery(t *testing.T) {
	var capturedURL *url.URL

	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`[{"id":1,"type":2,"project":{"id":9,"projectKey":"PROJ"},"content":{"id":100,"key_id":5,"summary":"hello"},"created":"2026-05-27T01:02:03Z"}]`,
			)),
		}, nil
	})

	activities, err := client.GetUserActivities(context.Background(), &ActivityListOptions{
		UserID:          123,
		ActivityTypeIDs: []int{1, 2, 3},
		MaxID:           500,
		Count:           100,
		Order:           "desc",
	})
	if err != nil {
		t.Fatalf("GetUserActivities returned error: %v", err)
	}

	if capturedURL == nil {
		t.Fatal("request was not captured")
	}
	if !strings.HasSuffix(capturedURL.Path, "/users/123/activities") {
		t.Fatalf("unexpected path: %s", capturedURL.Path)
	}

	q := capturedURL.Query()
	if got := q["activityTypeId[]"]; strings.Join(got, ",") != "1,2,3" {
		t.Fatalf("activityTypeId[] = %v, want [1 2 3]", got)
	}
	if got := q.Get("maxId"); got != "500" {
		t.Fatalf("maxId = %q, want 500", got)
	}
	if got := q.Get("count"); got != "100" {
		t.Fatalf("count = %q, want 100", got)
	}
	if got := q.Get("order"); got != "desc" {
		t.Fatalf("order = %q, want desc", got)
	}

	if len(activities) != 1 {
		t.Fatalf("len(activities) = %d, want 1", len(activities))
	}
	a := activities[0]
	if a.Type.Value != 2 {
		t.Fatalf("type = %d, want 2", a.Type.Value)
	}
	content, ok := a.Content.Get()
	if !ok {
		t.Fatal("content not set")
	}
	if keyID, ok := content.KeyID.Get(); !ok || keyID != 5 {
		t.Fatalf("content.key_id = %d (ok=%v), want 5", keyID, ok)
	}
	if p, ok := a.Project.Get(); !ok || p.ProjectKey.Value != "PROJ" {
		t.Fatalf("project.projectKey = %q (ok=%v), want PROJ", p.ProjectKey.Value, ok)
	}
}

func TestGetRecentlyViewedIssuesDecodes(t *testing.T) {
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/users/myself/recentlyViewedIssues") {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`[{"issue":{"id":7,"issueKey":"PROJ-7","summary":"viewed"},"updated":"2026-06-01T00:00:00Z"}]`,
			)),
		}, nil
	})

	viewed, err := client.GetRecentlyViewedIssues(context.Background(), &RecentlyViewedIssuesOptions{Order: "desc", Count: 100})
	if err != nil {
		t.Fatalf("GetRecentlyViewedIssues returned error: %v", err)
	}
	if len(viewed) != 1 {
		t.Fatalf("len(viewed) = %d, want 1", len(viewed))
	}
	is, ok := viewed[0].Issue.Get()
	if !ok || is.IssueKey.Value != "PROJ-7" {
		t.Fatalf("issue.issueKey = %q (ok=%v), want PROJ-7", is.IssueKey.Value, ok)
	}
}

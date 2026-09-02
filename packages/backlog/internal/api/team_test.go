package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGetTeamsEncodesQuery(t *testing.T) {
	var capturedURL *url.URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"name":"team1","members":[{"id":1,"userId":"admin","name":"Admin","roleType":1,"lang":"ja","mailAddress":"a@example.com"}],"displayOrder":0,"created":"2026-01-01T00:00:00Z","updated":"2026-01-02T00:00:00Z"}]`))
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	teams, err := client.GetTeams(context.Background(), &TeamListOptions{Order: "asc", Offset: 10, Count: 50})
	if err != nil {
		t.Fatalf("GetTeams returned error: %v", err)
	}

	if !strings.HasSuffix(capturedURL.Path, "/teams") {
		t.Fatalf("unexpected path: %s", capturedURL.Path)
	}
	q := capturedURL.Query()
	if q.Get("order") != "asc" || q.Get("offset") != "10" || q.Get("count") != "50" {
		t.Fatalf("unexpected query: %s", capturedURL.RawQuery)
	}
	if len(teams) != 1 || teams[0].Name != "team1" || len(teams[0].Members) != 1 {
		t.Fatalf("unexpected teams: %+v", teams)
	}
}

// TestGetTeamsOmitsQueryWhenOptionsZero verifies that leaving order/offset/
// count at their zero values (the "not specified" state for the --order/
// --offset/--count flags) sends no query string, so the API's own defaults
// apply instead of an explicit offset=0/count=0.
func TestGetTeamsOmitsQueryWhenOptionsZero(t *testing.T) {
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

	if _, err := client.GetTeams(context.Background(), &TeamListOptions{}); err != nil {
		t.Fatalf("GetTeams returned error: %v", err)
	}

	if capturedURL.RawQuery != "" {
		t.Fatalf("expected no query params for zero-valued options, got: %q", capturedURL.RawQuery)
	}
}

func TestGetTeamsErrorPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"message":"forbidden","code":11,"moreInfo":""}]}`))
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	_, err = client.GetTeams(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestGetTeamUsesTeamIDPath(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":5,"name":"team5","members":[],"displayOrder":1,"created":"2026-01-01T00:00:00Z","updated":"2026-01-02T00:00:00Z"}`))
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	team, err := client.GetTeam(context.Background(), 5)
	if err != nil {
		t.Fatalf("GetTeam returned error: %v", err)
	}
	if !strings.HasSuffix(capturedPath, "/teams/5") {
		t.Fatalf("path = %s, want suffix /teams/5", capturedPath)
	}
	if team.Name != "team5" {
		t.Fatalf("team.Name = %q, want team5", team.Name)
	}
}

func TestGetProjectTeamsUsesProjectPath(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":2,"name":"proj-team","members":[],"displayOrder":0,"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}]`))
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	teams, err := client.GetProjectTeams(context.Background(), "PROJ")
	if err != nil {
		t.Fatalf("GetProjectTeams returned error: %v", err)
	}
	if !strings.HasSuffix(capturedPath, "/projects/PROJ/teams") {
		t.Fatalf("path = %s, want suffix /projects/PROJ/teams", capturedPath)
	}
	if len(teams) != 1 || teams[0].Name != "proj-team" {
		t.Fatalf("unexpected teams: %+v", teams)
	}
}

func TestGetTeamErrorPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"message":"no such team","code":5,"moreInfo":""}]}`))
	}))
	defer server.Close()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = newRedirectingTransport(t, target)

	_, err = client.GetTeam(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

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

func newRelatedIssueTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient.Transport = newRedirectingTransport(t, target)
	return client
}

func TestGetRelatedIssues(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/issues/PROJ-1/relatedIssues") {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `[{"id":2,"issueKey":"PROJ-2","summary":"related summary","type":"RELATES"}]`)
		}))
		defer server.Close()

		client := newRelatedIssueTestClient(t, server)
		related, err := client.GetRelatedIssues(context.Background(), "PROJ-1")
		if err != nil {
			t.Fatalf("GetRelatedIssues returned error: %v", err)
		}
		if len(related) != 1 {
			t.Fatalf("len(related) = %d, want 1", len(related))
		}
		if related[0].IssueKey.Value != "PROJ-2" {
			t.Errorf("IssueKey = %q, want PROJ-2", related[0].IssueKey.Value)
		}
		if related[0].Type.Value != "RELATES" {
			t.Errorf("Type = %q, want RELATES", related[0].Type.Value)
		}
	})

	t.Run("error path", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"errors":[{"message":"No such issue","code":5}]}`)
		}))
		defer server.Close()

		client := newRelatedIssueTestClient(t, server)
		_, err := client.GetRelatedIssues(context.Background(), "PROJ-1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get related issues") {
			t.Errorf("error %q does not mention the operation", err.Error())
		}
	})
}

func TestAddRelatedIssue(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		var gotForm string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/issues/PROJ-1/relatedIssues") {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read body: %v", err)
			}
			gotForm = string(data)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":1,"issueKey":"PROJ-1","summary":"issue","type":"RELATES"}`)
		}))
		defer server.Close()

		client := newRelatedIssueTestClient(t, server)
		result, err := client.AddRelatedIssue(context.Background(), "PROJ-1", 2)
		if err != nil {
			t.Fatalf("AddRelatedIssue returned error: %v", err)
		}
		if result.IssueKey.Value != "PROJ-1" {
			t.Errorf("IssueKey = %q, want PROJ-1", result.IssueKey.Value)
		}
		form, err := url.ParseQuery(gotForm)
		if err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		if form.Get("relatedIssueId") != "2" {
			t.Errorf("relatedIssueId = %q, want 2", form.Get("relatedIssueId"))
		}
	})

	t.Run("error path", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"errors":[{"message":"invalid","code":1}]}`)
		}))
		defer server.Close()

		client := newRelatedIssueTestClient(t, server)
		_, err := client.AddRelatedIssue(context.Background(), "PROJ-1", 2)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to add related issue") {
			t.Errorf("error %q does not mention the operation", err.Error())
		}
	})
}

func TestDeleteRelatedIssue(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/issues/PROJ-1/relatedIssues/2") {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != http.MethodDelete {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":2,"issueKey":"PROJ-2","summary":"related","type":"RELATES"}`)
		}))
		defer server.Close()

		client := newRelatedIssueTestClient(t, server)
		result, err := client.DeleteRelatedIssue(context.Background(), "PROJ-1", 2)
		if err != nil {
			t.Fatalf("DeleteRelatedIssue returned error: %v", err)
		}
		if result.IssueKey.Value != "PROJ-2" {
			t.Errorf("IssueKey = %q, want PROJ-2", result.IssueKey.Value)
		}
	})

	t.Run("error path", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"errors":[{"message":"no such relation","code":1}]}`)
		}))
		defer server.Close()

		client := newRelatedIssueTestClient(t, server)
		_, err := client.DeleteRelatedIssue(context.Background(), "PROJ-1", 2)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete related issue") {
			t.Errorf("error %q does not mention the operation", err.Error())
		}
	})
}

package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCreateIssueEncodesBracketedArrayFields(t *testing.T) {
	var body string

	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		data, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		body = string(data)

		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	})

	_, err := client.CreateIssue(context.Background(), &CreateIssueInput{
		ProjectID:     1,
		Summary:       "summary",
		IssueTypeID:   2,
		PriorityID:    3,
		CategoryIDs:   []int{10, 11},
		VersionIDs:    []int{20},
		MilestoneIDs:  []int{30},
		AttachmentIDs: []int{40, 41},
	})
	if err != nil {
		t.Fatalf("CreateIssue returned error: %v", err)
	}

	form, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}

	assertValues := func(key string, want []string) {
		t.Helper()
		got := form[key]
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s = %v, want %v", key, got, want)
		}
	}

	assertValues("categoryId[]", []string{"10", "11"})
	assertValues("versionId[]", []string{"20"})
	assertValues("milestoneId[]", []string{"30"})
	assertValues("attachmentId[]", []string{"40", "41"})

	for _, key := range []string{"categoryId", "versionId", "milestoneId", "attachmentId"} {
		if _, ok := form[key]; ok {
			t.Fatalf("unexpected unbracketed key %q in form: %v", key, form)
		}
	}
}

func TestUpdateIssueEncodesBracketedArrayFields(t *testing.T) {
	var body string

	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		data, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		body = string(data)

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	})

	_, err := client.UpdateIssue(context.Background(), "PROJ-1", &UpdateIssueInput{
		CategoryIDs:   []int{10, 11},
		VersionIDs:    []int{20},
		MilestoneIDs:  []int{30},
		AttachmentIDs: []int{40, 41},
	})
	if err != nil {
		t.Fatalf("UpdateIssue returned error: %v", err)
	}

	form, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}

	assertValues := func(key string, want []string) {
		t.Helper()
		got := form[key]
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s = %v, want %v", key, got, want)
		}
	}

	assertValues("categoryId[]", []string{"10", "11"})
	assertValues("versionId[]", []string{"20"})
	assertValues("milestoneId[]", []string{"30"})
	assertValues("attachmentId[]", []string{"40", "41"})

	for _, key := range []string{"categoryId", "versionId", "milestoneId", "attachmentId"} {
		if _, ok := form[key]; ok {
			t.Fatalf("unexpected unbracketed key %q in form: %v", key, form)
		}
	}
}

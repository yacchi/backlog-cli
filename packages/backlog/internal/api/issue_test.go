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

// TestCreateIssueOmitsParentIssueIdFieldWhenUnset is an adversarial test derived
// from T3's contract requirement: "When the flag is empty or absent, the request
// must be byte-for-byte what it is today (no parentIssueId field emitted at all —
// not an empty string, not 0)."
func TestCreateIssueOmitsParentIssueIdFieldWhenUnset(t *testing.T) {
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
		ProjectID:   1,
		Summary:     "summary",
		IssueTypeID: 2,
		PriorityID:  3,
		// ParentIssueID intentionally left at its zero value.
	})
	if err != nil {
		t.Fatalf("CreateIssue returned error: %v", err)
	}

	form, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}

	if _, ok := form["parentIssueId"]; ok {
		t.Fatalf("parentIssueId field must not be emitted when --parent is absent, got form: %v", form)
	}
}

// TestCreateIssueEmitsResolvedParentIssueId is an adversarial test derived from
// T3's contract requirement: "--parent PROJ-1 sends parentIssueId with the
// resolved numeric id."
func TestCreateIssueEmitsResolvedParentIssueId(t *testing.T) {
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
		ParentIssueID: 999,
	})
	if err != nil {
		t.Fatalf("CreateIssue returned error: %v", err)
	}

	form, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}

	if got := form.Get("parentIssueId"); got != "999" {
		t.Fatalf("parentIssueId = %q, want %q", got, "999")
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

// TestUpdateIssueOmitsParentIssueIdFieldWhenUnset verifies that a plain
// UpdateIssueInput with neither ParentIssueID nor RemoveParent set stays on
// the generated-client path and never emits a parentIssueId field, since the
// ogen-generated UpdateIssueReq has no such field.
func TestUpdateIssueOmitsParentIssueIdFieldWhenUnset(t *testing.T) {
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

	title := "new title"
	_, err := client.UpdateIssue(context.Background(), "PROJ-1", &UpdateIssueInput{
		Summary: &title,
	})
	if err != nil {
		t.Fatalf("UpdateIssue returned error: %v", err)
	}

	form, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if _, ok := form["parentIssueId"]; ok {
		t.Fatalf("parentIssueId field must not be emitted when unset, got form: %v", form)
	}
	if got := form.Get("summary"); got != title {
		t.Fatalf("summary = %q, want %q", got, title)
	}
}

// TestUpdateIssueSetsParentIssueId is an adversarial test derived from T3's
// contract requirement: "--parent sets it" (edit tests, Part B).
func TestUpdateIssueSetsParentIssueId(t *testing.T) {
	var body string
	var method string

	client := NewClient("example.backlog.jp", "", WithAPIKey("test"))
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		method = req.Method
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

	parentID := 999
	_, err := client.UpdateIssue(context.Background(), "PROJ-1", &UpdateIssueInput{
		ParentIssueID: &parentID,
	})
	if err != nil {
		t.Fatalf("UpdateIssue returned error: %v", err)
	}

	if method != http.MethodPatch {
		t.Fatalf("method = %q, want %q", method, http.MethodPatch)
	}

	form, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if got := form.Get("parentIssueId"); got != "999" {
		t.Fatalf("parentIssueId = %q, want %q", got, "999")
	}
}

// TestUpdateIssueRemoveParentClearsParentIssueId is an adversarial test
// derived from T3's contract requirement: "--remove-parent clears it" (edit
// tests, Part B), modeled on how --remove-milestone clears MilestoneIDs by
// sending an explicit empty value rather than omitting the field.
func TestUpdateIssueRemoveParentClearsParentIssueId(t *testing.T) {
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
		RemoveParent: true,
	})
	if err != nil {
		t.Fatalf("UpdateIssue returned error: %v", err)
	}

	form, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if _, ok := form["parentIssueId"]; !ok {
		t.Fatalf("parentIssueId field must be present (empty) to clear the parent, got form: %v", form)
	}
	if got := form.Get("parentIssueId"); got != "" {
		t.Fatalf("parentIssueId = %q, want empty string", got)
	}
}

// TestUpdateIssueWithParentAlsoEncodesOtherFields verifies the hand-written
// parent-aware update path (updateIssueWithParent) preserves the other
// fields' wire encoding, matching the generated client's field names.
func TestUpdateIssueWithParentAlsoEncodesOtherFields(t *testing.T) {
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

	title := "new title"
	parentID := 5
	_, err := client.UpdateIssue(context.Background(), "PROJ-1", &UpdateIssueInput{
		Summary:       &title,
		CategoryIDs:   []int{10, 11},
		ParentIssueID: &parentID,
	})
	if err != nil {
		t.Fatalf("UpdateIssue returned error: %v", err)
	}

	form, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if got := form.Get("summary"); got != title {
		t.Fatalf("summary = %q, want %q", got, title)
	}
	if got := form["categoryId[]"]; strings.Join(got, ",") != "10,11" {
		t.Fatalf("categoryId[] = %v, want [10 11]", got)
	}
	if got := form.Get("parentIssueId"); got != "5" {
		t.Fatalf("parentIssueId = %q, want %q", got, "5")
	}
}

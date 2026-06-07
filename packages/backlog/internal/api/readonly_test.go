package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadOnlyTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := &ReadOnlyTransport{Base: http.DefaultTransport}

	tests := []struct {
		name      string
		method    string
		mode      string
		wantError bool
	}{
		{"GET allowed in read-only", http.MethodGet, "read-only", false},
		{"HEAD allowed in read-only", http.MethodHead, "read-only", false},
		{"OPTIONS allowed in read-only", http.MethodOptions, "read-only", false},
		{"POST blocked in read-only", http.MethodPost, "read-only", true},
		{"PUT blocked in read-only", http.MethodPut, "read-only", true},
		{"PATCH blocked in read-only", http.MethodPatch, "read-only", true},
		{"DELETE blocked in read-only", http.MethodDelete, "read-only", true},
		{"POST allowed when not read-only", http.MethodPost, "", false},
		{"DELETE allowed when not read-only", http.MethodDelete, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BACKLOG_ACCESS_MODE", tt.mode)

			req, err := http.NewRequest(tt.method, server.URL+"/test", nil)
			if err != nil {
				t.Fatal(err)
			}

			resp, err := transport.RoundTrip(req)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error for %s in mode %q, got nil", tt.method, tt.mode)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for %s in mode %q: %v", tt.method, tt.mode, err)
				}
				if resp != nil {
					_ = resp.Body.Close()
				}
			}
		})
	}
}

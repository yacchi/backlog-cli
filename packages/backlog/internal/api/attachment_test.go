package api

import (
	"net/http"
	"testing"
)

func makeResp(contentDisposition string) *http.Response {
	h := http.Header{}
	if contentDisposition != "" {
		h.Set("Content-Disposition", contentDisposition)
	}
	return &http.Response{Header: h}
}

func TestExtractFilename(t *testing.T) {
	tests := []struct {
		name string
		cd   string
		want string
	}{
		{"empty", "", ""},
		{"simple", `attachment; filename="report.pdf"`, "report.pdf"},
		{"unquoted", `attachment; filename=report.pdf`, "report.pdf"},
		{"rfc5987 utf8", `attachment; filename*=UTF-8''%E3%83%86%E3%82%B9%E3%83%88.pdf`, "テスト.pdf"},
		{"filename* wins", `attachment; filename="ascii.pdf"; filename*=UTF-8''%E6%97%A5%E6%9C%AC%E8%AA%9E.pdf`, "日本語.pdf"},
		{"no params", "attachment", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFilename(makeResp(tt.cd))
			if got != tt.want {
				t.Errorf("extractFilename(%q) = %q, want %q", tt.cd, got, tt.want)
			}
		})
	}
}

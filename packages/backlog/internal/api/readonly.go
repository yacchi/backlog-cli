package api

import (
	"fmt"
	"net/http"
	"os"
)

// ReadOnlyTransport は BACKLOG_ACCESS_MODE=read-only 時に
// GET/HEAD/OPTIONS 以外のHTTPメソッドをブロックする RoundTripper。
type ReadOnlyTransport struct {
	Base http.RoundTripper
}

func (t *ReadOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if os.Getenv("BACKLOG_ACCESS_MODE") == "read-only" {
		switch req.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			return nil, fmt.Errorf("HTTP %s is not allowed in read-only mode", req.Method)
		}
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

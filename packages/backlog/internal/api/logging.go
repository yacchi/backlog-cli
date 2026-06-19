package api

import (
	"net/http"
	"time"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/debug"
)

type LoggingTransport struct {
	Base http.RoundTripper
}

func (t *LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !debug.IsEnabled() {
		return t.base().RoundTrip(req)
	}

	start := time.Now()

	reqSize := req.ContentLength
	if reqSize < 0 {
		reqSize = 0
	}

	resp, err := t.base().RoundTrip(req)
	if err != nil {
		debug.Log("http request failed",
			"method", req.Method,
			"url", req.URL.String(),
			"request_bytes", reqSize,
			"error", err,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return nil, err
	}

	respSize := resp.ContentLength
	if respSize < 0 {
		respSize = 0
	}

	debug.Log("http request",
		"method", req.Method,
		"url", req.URL.String(),
		"status", resp.StatusCode,
		"request_bytes", reqSize,
		"response_bytes", respSize,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return resp, nil
}

func (t *LoggingTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

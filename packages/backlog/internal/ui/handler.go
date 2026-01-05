package ui

import (
	"io/fs"
	"net/http"
	"strings"
)

// SPAHandler serves static assets with an index.html fallback for client routing.
func SPAHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		f, err := assets.Open(path)
		if err == nil {
			defer func() { _ = f.Close() }()
			stat, statErr := f.Stat()
			if statErr == nil && !stat.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		content, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(content)
	})
}

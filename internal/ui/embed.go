//go:build !dev

package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Assets returns the production static file system.
func Assets() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

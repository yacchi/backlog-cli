//go:build dev

package web

import (
	"io/fs"
	"os"
)

// Assets returns the development static file system.
func Assets() (fs.FS, error) {
	return os.DirFS("packages/web/dist"), nil
}

//go:build dev

package ui

import (
	"io/fs"
	"os"
)

// Assets returns the development static file system.
func Assets() (fs.FS, error) {
	return os.DirFS("internal/ui/dist"), nil
}

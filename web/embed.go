// Package webassets contains the embedded browser application.
package webassets

import (
	"embed"
	"io/fs"
)

// Files is the built browser application served by niridesk. Frontend source
// is built with pnpm and Vite; dist/ is generated and mostly ignored by Git.
//
//go:embed all:dist
var Files embed.FS

// DistFS returns the build output as a rooted filesystem.
func DistFS() (fs.FS, error) {
	return fs.Sub(Files, "dist")
}

// HasFrontend reports whether the binary was built with frontend assets.
func HasFrontend() bool {
	_, err := fs.Stat(Files, "dist/index.html")
	return err == nil
}

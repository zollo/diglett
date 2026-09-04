// Package web holds Diglett's embedded static web UI assets.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var embedded embed.FS

// Static returns the filesystem rooted at the static asset directory, suitable
// for serving with http.FileServer.
func Static() fs.FS {
	sub, err := fs.Sub(embedded, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

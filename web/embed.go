// Package web holds the web app. Its Go side embeds the Vite build in dist,
// since go:embed cannot reach files outside the package directory.
package web

import (
	"embed"
	"io/fs"
)

// dist keeps a committed .gitkeep so the package builds before the web app
// is built; the all: prefix embeds it, and the build writes the rest.
//
//go:embed all:dist
var dist embed.FS

// Dist returns the built web app, laid out as in dist.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// fs.Sub fails only for an invalid path, and "dist" is valid.
		panic(err)
	}

	return sub
}

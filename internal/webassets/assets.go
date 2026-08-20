// Package webassets provides Ivy's embedded production browser client.
package webassets

import (
	"embed"
	"io/fs"
)

// embedded contains the exact Vite production output committed under dist.
//
//go:embed dist
var embedded embed.FS

// FS returns the production browser client rooted at index.html.
func FS() fs.FS {
	assets, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic("ivy: invalid embedded web asset root: " + err.Error())
	}
	return assets
}

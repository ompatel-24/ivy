package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ompatel-24/ivy/internal/webassets"
)

func resolveWebAssets(explicitRoot string) (fs.FS, error) {
	if explicitRoot != "" {
		return webAssetsAt(explicitRoot)
	}
	return webassets.FS(), nil
}

func webAssetsAt(root string) (fs.FS, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve web asset directory %q: %w", root, err)
	}
	info, err := os.Stat(filepath.Join(absoluteRoot, "index.html"))
	if err != nil {
		return nil, fmt.Errorf("read web asset directory %q: %w", root, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("web asset directory %q has no regular index.html", root)
	}
	return os.DirFS(absoluteRoot), nil
}

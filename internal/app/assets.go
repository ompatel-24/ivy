package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func resolveWebAssets(explicitRoot string) (fs.FS, error) {
	if explicitRoot != "" {
		return webAssetsAt(explicitRoot)
	}

	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate Ivy binary: %w", err)
	}
	assets, err := webAssetsAt(filepath.Join(filepath.Dir(executable), "web"))
	if err == nil {
		return assets, nil
	}
	return nil, fmt.Errorf("web assets not found beside the Ivy binary; run 'make build' or set IVY_WEB_DIR")
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

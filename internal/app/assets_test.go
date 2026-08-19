package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveWebAssetsExplicitRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ivy"), 0o644); err != nil {
		t.Fatal(err)
	}

	assets, err := resolveWebAssets(root)
	if err != nil {
		t.Fatalf("resolveWebAssets(): %v", err)
	}
	data, err := fs.ReadFile(assets, "index.html")
	if err != nil || string(data) != "ivy" {
		t.Fatalf("resolved index = (%q, %v)", data, err)
	}
}

func TestResolveWebAssetsRejectsMissingExplicitRoot(t *testing.T) {
	_, err := resolveWebAssets(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "read web asset directory") {
		t.Fatalf("resolveWebAssets() error = %v", err)
	}
}

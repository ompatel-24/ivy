package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ompatel-24/rome/internal/server"
)

func TestResolveWebAssetsUsesEmbeddedDefault(t *testing.T) {
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	assets, err := resolveWebAssets("")
	if err != nil {
		t.Fatalf("resolveWebAssets(): %v", err)
	}
	if err := server.ValidateWebAssets(assets); err != nil {
		t.Fatalf("embedded assets are invalid: %v", err)
	}
	index, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		t.Fatalf("read embedded index: %v", err)
	}
	if !strings.Contains(string(index), `<main id="app"`) {
		t.Fatalf("embedded index is not the Rome client: %q", index)
	}
}

func TestResolveWebAssetsExplicitRootTakesPrecedence(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("rome"), 0o644); err != nil {
		t.Fatal(err)
	}

	assets, err := resolveWebAssets(root)
	if err != nil {
		t.Fatalf("resolveWebAssets(): %v", err)
	}
	data, err := fs.ReadFile(assets, "index.html")
	if err != nil || string(data) != "rome" {
		t.Fatalf("resolved index = (%q, %v)", data, err)
	}
}

func TestResolveWebAssetsRejectsMissingExplicitRoot(t *testing.T) {
	_, err := resolveWebAssets(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "read web asset directory") {
		t.Fatalf("resolveWebAssets() error = %v", err)
	}
}

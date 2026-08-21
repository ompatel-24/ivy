package webassets_test

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/ompatel-24/rome/internal/server"
	"github.com/ompatel-24/rome/internal/webassets"
)

var assetReferencePattern = regexp.MustCompile(`(?:src|href)=["'](/assets/[^"'?#]+)["']`)

func TestEmbeddedAssetsAreComplete(t *testing.T) {
	assets := webassets.FS()
	if err := server.ValidateWebAssets(assets); err != nil {
		t.Fatalf("ValidateWebAssets(): %v", err)
	}
	index, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	references := assetReferencePattern.FindAllSubmatch(index, -1)
	if len(references) == 0 {
		t.Fatal("embedded index contains no asset references")
	}
	for _, reference := range references {
		name := strings.TrimPrefix(string(reference[1]), "/")
		info, err := fs.Stat(assets, name)
		if err != nil {
			t.Errorf("stat %q: %v", name, err)
			continue
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			t.Errorf("embedded asset %q is not a non-empty regular file", name)
		}
	}
}

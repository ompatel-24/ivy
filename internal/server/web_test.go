package server

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestValidateWebAssets(t *testing.T) {
	if err := ValidateWebAssets(nil); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("ValidateWebAssets(nil) error = %v", err)
	}

	tests := []struct {
		name    string
		assets  fstest.MapFS
		wantErr string
	}{
		{name: "valid", assets: testWebAssets()},
		{name: "missing index", assets: fstest.MapFS{}, wantErr: "read web index"},
		{name: "directory index", assets: fstest.MapFS{"index.html": &fstest.MapFile{Mode: fs.ModeDir | 0o755}}, wantErr: "not a regular file"},
		{name: "oversize index", assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: make([]byte, maxIndexBytes+1)}}, wantErr: "exceeds"},
		{name: "no built assets", assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}, wantErr: "no built asset references"},
		{
			name: "missing referenced asset",
			assets: fstest.MapFS{
				"index.html": &fstest.MapFile{Data: []byte(`<script src="/assets/missing.js"></script>`)},
			},
			wantErr: "read web asset",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateWebAssets(test.assets)
			if test.wantErr == "" && err != nil {
				t.Fatalf("ValidateWebAssets(): %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("ValidateWebAssets() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

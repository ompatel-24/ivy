package server

import (
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
)

const (
	maxIndexBytes    = 1024 * 1024
	maxWebAssetBytes = 8 * 1024 * 1024
)

var webAssetReferencePattern = regexp.MustCompile(`(?:src|href)=["'](/assets/[^"'?#]+)["']`)

// ValidateWebAssets verifies the minimum filesystem contract required by the
// mobile client before Rome starts a child process.
func ValidateWebAssets(web fs.FS) error {
	if web == nil {
		return fmt.Errorf("web assets are unavailable")
	}
	info, err := fs.Stat(web, "index.html")
	if err != nil {
		return fmt.Errorf("read web index: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("web index is not a regular file")
	}
	if info.Size() > maxIndexBytes {
		return fmt.Errorf("web index exceeds %d bytes", maxIndexBytes)
	}
	index, err := fs.ReadFile(web, "index.html")
	if err != nil {
		return fmt.Errorf("read web index: %w", err)
	}
	references := webAssetReferencePattern.FindAllSubmatch(index, -1)
	if len(references) == 0 {
		return fmt.Errorf("web index contains no built asset references")
	}
	for _, reference := range references {
		name := strings.TrimPrefix(string(reference[1]), "/")
		asset, assetErr := fs.Stat(web, name)
		if assetErr != nil {
			return fmt.Errorf("read web asset %q: %w", name, assetErr)
		}
		if !asset.Mode().IsRegular() {
			return fmt.Errorf("web asset %q is not a regular file", name)
		}
		if asset.Size() > maxWebAssetBytes {
			return fmt.Errorf("web asset %q exceeds %d bytes", name, maxWebAssetBytes)
		}
	}
	return nil
}

func (s *Server) handleSessionPage(w http.ResponseWriter, request *http.Request) {
	if request.PathValue("id") != s.sessionID {
		s.serveWebFile(w, request, "index.html", http.StatusNotFound)
		return
	}
	if _, ok := s.manager.Get(s.sessionID); !ok {
		s.serveWebFile(w, request, "index.html", http.StatusNotFound)
		return
	}
	s.serveWebFile(w, request, "index.html", http.StatusOK)
}

func (s *Server) handleWebAsset(w http.ResponseWriter, request *http.Request) {
	assetPath := request.PathValue("path")
	if assetPath == "" || !fs.ValidPath(assetPath) || strings.HasSuffix(assetPath, "/") {
		http.NotFound(w, request)
		return
	}
	s.serveWebFile(w, request, path.Join("assets", assetPath), http.StatusOK)
}

func (s *Server) serveWebFile(w http.ResponseWriter, request *http.Request, name string, status int) {
	file, err := s.web.Open(name)
	if err != nil {
		http.NotFound(w, request)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, request)
		return
	}
	if info.Size() > maxWebAssetBytes {
		http.NotFound(w, request)
		return
	}
	data, err := fs.ReadFile(s.web, name)
	if err != nil {
		http.NotFound(w, request)
		return
	}

	setWebSecurityHeaders(w.Header())
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func setWebSecurityHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self' ws: wss:; img-src 'self' data:; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	header.Set("Cross-Origin-Resource-Policy", "same-origin")
	header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}

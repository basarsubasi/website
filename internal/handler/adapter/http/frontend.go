package httpadapter

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	homePath                   = "/"
	photosPath                 = "/photos"
	monochromePhotosPath       = "/bw-photos"
	colorPhotosPath            = "/color-photos"
	blogPath                   = "/blog"
	adminPath                  = "/admin"
	photoPathPrefix            = "/photo/"
	blogPathPrefix             = "/blog/"
	indexFilename              = "index.html"
	frontendUnavailableMessage = "Frontend not built. Run npm ci && npm run build in ui/."
	frontendCacheControlHeader = "Cache-Control"
	noCache                    = "no-cache"
)

func (s *Server) frontend(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	root, err := os.OpenRoot(s.cfg.FrontendDir)
	if err == nil {
		defer root.Close()
		name := strings.TrimPrefix(p, homePath)
		if serveFrontendFile(w, r, root, name, false) {
			return
		}
	}
	if !isFrontendRoute(p) {
		http.NotFound(w, r)
		return
	}
	if root == nil || !serveFrontendFile(w, r, root, indexFilename, true) {
		http.Error(w, frontendUnavailableMessage, http.StatusServiceUnavailable)
	}
}

func isFrontendRoute(path string) bool {
	switch path {
	case homePath, photosPath, monochromePhotosPath, colorPhotosPath, blogPath, adminPath:
		return true
	}
	return strings.HasPrefix(path, photoPathPrefix) || strings.HasPrefix(path, blogPathPrefix)
}

func serveFrontendFile(w http.ResponseWriter, r *http.Request, root *os.Root, name string, fallback bool) bool {
	local, err := filepath.Localize(name)
	if err != nil {
		return false
	}
	file, err := root.Open(local)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if fallback {
		w.Header().Set(frontendCacheControlHeader, noCache)
	}
	if strings.HasSuffix(r.URL.Path, homePath+indexFilename) {
		// ServeFileFS preserves the canonical directory URL for explicit index requests.
		http.ServeFileFS(w, r, root.FS(), indexFilename)
		return true
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	return true
}

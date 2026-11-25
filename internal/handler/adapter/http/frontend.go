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
	path := filepath.Join(s.cfg.FrontendDir, filepath.FromSlash(r.URL.Path))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	// Only application routes receive the SPA fallback; missing assets remain 404s.
	p := r.URL.Path
	if p != homePath && p != photosPath && p != monochromePhotosPath && p != colorPhotosPath && p != blogPath && p != adminPath && !strings.HasPrefix(p, photoPathPrefix) && !strings.HasPrefix(p, blogPathPrefix) {
		http.NotFound(w, r)
		return
	}
	index := filepath.Join(s.cfg.FrontendDir, indexFilename)
	if _, err := os.Stat(index); err != nil {
		http.Error(w, frontendUnavailableMessage, 503)
		return
	}
	w.Header().Set(frontendCacheControlHeader, noCache)
	http.ServeFile(w, r, index)
}

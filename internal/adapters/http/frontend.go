package httpadapter

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) frontend(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(s.cfg.FrontendDir, filepath.FromSlash(r.URL.Path))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	// Only application routes receive the SPA fallback; missing assets remain 404s.
	p := r.URL.Path
	if p != "/" && p != "/photos" && p != "/bw-photos" && p != "/color-photos" && p != "/blog" && p != "/admin" && !strings.HasPrefix(p, "/photo/") && !strings.HasPrefix(p, "/blog/") {
		http.NotFound(w, r)
		return
	}
	index := filepath.Join(s.cfg.FrontendDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		http.Error(w, "Frontend not built. Run npm ci && npm run build in frontend/.", 503)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, index)
}

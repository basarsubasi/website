package httpadapter

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/usecase"
)

type Config struct {
	Password       string
	FrontendDir    string
	SecureCookies  bool
	TrustedProxies []netip.Prefix
}

type Server struct {
	service  *usecase.Service
	cfg      Config
	mu       sync.Mutex
	sessions map[[32]byte]time.Time
	logins   loginLimiter
}

func New(service *usecase.Service, cfg Config) http.Handler {
	s := &Server{service: service, cfg: cfg, sessions: make(map[[32]byte]time.Time)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if err := service.Health(r.Context()); err != nil {
			fail(w, 503, "Object storage unavailable")
			return
		}
		reply(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/auth/session", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]bool{"authenticated": s.authenticated(r)})
	})
	mux.HandleFunc("GET /api/images", s.listImages)
	mux.HandleFunc("GET /api/images/{uuid}", s.getImage)
	mux.HandleFunc("GET /api/posts", s.listPosts)
	mux.HandleFunc("GET /api/posts/{slug}", s.getPost)
	mux.Handle("GET /api/admin/posts", s.admin(http.HandlerFunc(s.listPosts)))
	mux.Handle("POST /api/admin/images", s.admin(http.HandlerFunc(s.uploadImage)))
	mux.Handle("PUT /api/admin/images/{uuid}", s.admin(http.HandlerFunc(s.saveImage)))
	mux.Handle("DELETE /api/admin/images/{uuid}", s.admin(http.HandlerFunc(s.deleteImage)))
	mux.Handle("POST /api/admin/posts", s.admin(http.HandlerFunc(s.savePost)))
	mux.Handle("PUT /api/admin/posts/{id}", s.admin(http.HandlerFunc(s.savePost)))
	mux.Handle("DELETE /api/admin/posts/{id}", s.admin(http.HandlerFunc(s.deletePost)))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "Endpoint not found") })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.frontend(w, r)
	})
	protection := http.NewCrossOriginProtection()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		protection.Handler(mux).ServeHTTP(w, r)
	})
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(v)
	}
}
func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]string{"error": message})
}
func appError(w http.ResponseWriter, err error) {
	var invalid *domain.ValidationError
	var storage *domain.StorageError
	switch {
	case errors.As(err, &invalid):
		fail(w, 400, invalid.Error())
	case errors.Is(err, domain.ErrNotFound):
		fail(w, 404, "Not found")
	case errors.Is(err, domain.ErrConflict):
		fail(w, 409, "Content changed or identifier already exists; reload and retry")
	case errors.Is(err, domain.ErrUncertain):
		fail(w, 503, "Write outcome is uncertain; reload before retrying")
	case errors.Is(err, domain.ErrBusy):
		w.Header().Set("Retry-After", "1")
		fail(w, 503, "Storage is busy; retry shortly")
	case errors.Is(err, domain.ErrUnavailable):
		fail(w, 503, "Object storage is not configured")
	case errors.Is(err, domain.ErrTooLarge):
		fail(w, 413, err.Error())
	case errors.As(err, &storage):
		log.Printf("storage: %v", storage.Err)
		fail(w, 502, storage.Error())
	default:
		log.Printf("application: %v", err)
		fail(w, 500, "Operation failed")
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, "Invalid JSON body (maximum 1 MiB)")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, 400, "Expected a single JSON object")
		return false
	}
	return true
}
func pagination(w http.ResponseWriter, r *http.Request, defaultLimit int64) (int64, int64, bool) {
	page, limit := int64(1), defaultLimit
	for key, target := range map[string]*int64{"page": &page, "limit": &limit} {
		if raw := r.URL.Query().Get(key); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n < 1 {
				fail(w, 400, "page and limit must be positive integers")
				return 0, 0, false
			}
			*target = n
		}
	}
	if limit > 100 || page > 1000000 {
		fail(w, 400, "Pagination exceeds maximum (limit 100, page 1000000)")
		return 0, 0, false
	}
	return page, limit, true
}
func (s *Server) authenticated(r *http.Request) bool {
	cookie, err := r.Cookie("admin_session")
	if err != nil {
		return false
	}
	key := sha256.Sum256([]byte(cookie.Value))
	s.mu.Lock()
	defer s.mu.Unlock()
	expires, ok := s.sessions[key]
	if !ok {
		return false
	}
	if time.Now().After(expires) {
		delete(s.sessions, key)
		return false
	}
	return true
}
func (s *Server) admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authenticated(r) {
			fail(w, 401, "Please sign in")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Password == "" {
		fail(w, 503, "Admin login is disabled; configure ADMIN_PASSWORD")
		return
	}
	now := time.Now()
	limited := !s.logins.allow(clientAddress(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), s.cfg.TrustedProxies), now)
	if limited {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "Too many login attempts. Try again in a minute.")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	got, want := sha256.Sum256([]byte(body.Password)), sha256.Sum256([]byte(s.cfg.Password))
	if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
		fail(w, 401, "Invalid password")
		return
	}
	token := uuid.NewV4().String()
	expires := now.Add(6 * time.Hour)
	s.mu.Lock()
	for key, expiry := range s.sessions {
		if now.After(expiry) {
			delete(s.sessions, key)
		}
	}
	if old, err := r.Cookie("admin_session"); err == nil {
		delete(s.sessions, sha256.Sum256([]byte(old.Value)))
	}
	s.sessions[sha256.Sum256([]byte(token))] = expires
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "admin_session", Value: token, Path: "/api", HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: 21600, Expires: expires})
	reply(w, 200, map[string]bool{"authenticated": true})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("admin_session"); err == nil {
		s.mu.Lock()
		delete(s.sessions, sha256.Sum256([]byte(cookie.Value)))
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "admin_session", Path: "/api", HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	reply(w, 204, nil)
}
func (s *Server) listImages(w http.ResponseWriter, r *http.Request) {
	page, limit, ok := pagination(w, r, 9)
	if !ok {
		return
	}
	data, err := s.service.ListImages(r.Context(), domain.Pagination{Page: page, Limit: limit}, r.URL.Query().Get("color"))
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, 200, data)
}
func (s *Server) getImage(w http.ResponseWriter, r *http.Request) {
	data, err := s.service.GetImage(r.Context(), r.PathValue("uuid"))
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, 200, map[string]any{"data": data})
}
func (s *Server) listPosts(w http.ResponseWriter, r *http.Request) {
	page, limit, ok := pagination(w, r, 10)
	if !ok {
		return
	}
	data, err := s.service.ListPosts(r.Context(), domain.Pagination{Page: page, Limit: limit}, r.URL.Path == "/api/admin/posts")
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, 200, data)
}
func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	data, err := s.service.GetPost(r.Context(), r.PathValue("slug"))
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, 200, map[string]any{"data": data})
}
func (s *Server) savePost(w http.ResponseWriter, r *http.Request) {
	var p domain.PostInput
	if !decode(w, r, &p) {
		return
	}
	post, err := s.service.SavePost(r.Context(), r.PathValue("id"), p)
	if err != nil {
		appError(w, err)
		return
	}
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	reply(w, status, map[string]any{"data": post})
}
func (s *Server) deletePost(w http.ResponseWriter, r *http.Request) {
	if err := s.service.DeletePost(r.Context(), r.PathValue("id")); err != nil {
		appError(w, err)
		return
	}
	reply(w, 204, nil)
}
func (s *Server) frontend(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(s.cfg.FrontendDir, filepath.FromSlash(r.URL.Path))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	// Only application routes receive the SPA fallback; missing assets remain 404s.
	p := r.URL.Path
	if p != "/" && p != "/bw-photos" && p != "/color-photos" && p != "/blog" && p != "/admin" && !strings.HasPrefix(p, "/image/") && !strings.HasPrefix(p, "/blog/") {
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

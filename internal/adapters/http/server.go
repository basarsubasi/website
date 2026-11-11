package httpadapter

import (
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/basarsubasi/website/internal/usecase"
)

type Config struct {
	Password       string
	FrontendDir    string
	SecureCookies  bool
	TrustedProxies []netip.Prefix
	TrustFlyProxy  bool
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
			fail(w, http.StatusServiceUnavailable, "Object storage unavailable")
			return
		}
		reply(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/auth/session", func(w http.ResponseWriter, r *http.Request) {
		reply(w, http.StatusOK, map[string]bool{"authenticated": s.authenticated(r)})
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
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, http.StatusNotFound, "Endpoint not found") })
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

package httpadapter

import (
	"context"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/basarsubasi/website/internal/module/blog"
	"github.com/basarsubasi/website/internal/module/gallery"
	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	healthRoute              = "GET /api/health"
	unhealthyMessage         = "Object storage unavailable"
	statusField              = "status"
	healthyStatus            = "ok"
	loginRoute               = "POST /api/auth/login"
	logoutRoute              = "POST /api/auth/logout"
	sessionRoute             = "GET /api/auth/session"
	serverAuthenticatedField = "authenticated"
	listImagesRoute          = "GET /api/images"
	getImageRoute            = "GET /api/images/{uuid}"
	listPostsRoute           = "GET /api/posts"
	getPostRoute             = "GET /api/posts/{slug}"
	adminAPIPath             = "/api/admin"
	adminAPIPrefix           = adminAPIPath + "/"
	listAdminPostsRoute      = "GET /api/admin/posts"
	uploadPhotoRoute         = "POST /api/admin/images"
	updatePhotoRoute         = "PUT /api/admin/images/{uuid}"
	movePhotoRoute           = "POST /api/admin/images/{uuid}/move"
	deletePhotoRoute         = "DELETE /api/admin/images/{uuid}"
	deleteBlogImageRoute     = "DELETE /api/admin/blog/images/{filename}"
	uploadBlogImageRoute     = "POST /api/admin/blog/images"
	createPostRoute          = "POST /api/admin/posts"
	updatePostRoute          = "PUT /api/admin/posts/{id}"
	deletePostRoute          = "DELETE /api/admin/posts/{id}"
	apiPrefix                = "/api/"
	unknownEndpointMessage   = "Endpoint not found"
	rootPath                 = "/"
	allowHeader              = "Allow"
	frontendAllowedMethods   = "GET, HEAD"
	methodNotAllowedMessage  = "Method not allowed"
	contentTypeOptionsHeader = "X-Content-Type-Options"
	noSniff                  = "nosniff"
	referrerPolicyHeader     = "Referrer-Policy"
	strictOriginPolicy       = "strict-origin-when-cross-origin"
	frameOptionsHeader       = "X-Frame-Options"
	denyFraming              = "DENY"
	serverCacheControlHeader = "Cache-Control"
	noStore                  = "no-store"
)

type Config struct {
	PhotoOrigin, BlogOrigin media.Origin
	Password                string
	FrontendDir             string
	SecureCookies           bool
	TrustedProxies          []netip.Prefix
	TrustFlyProxy           bool
	MaxRequests             int
	MaxUploads              int
}

type Server struct {
	gallery  *gallery.Module
	blog     *blog.Module
	cfg      Config
	mu       sync.Mutex
	sessions map[[32]byte]time.Time
	logins   loginLimiter
}

func New(gallery *gallery.Module, blog *blog.Module, health func(context.Context) error, cfg Config) http.Handler {
	if cfg.MaxRequests <= 0 {
		cfg.MaxRequests = DefaultMaxRequests
	}
	if cfg.MaxUploads <= 0 {
		cfg.MaxUploads = DefaultMaxUploads
	}
	s := &Server{gallery: gallery, blog: blog, cfg: cfg, sessions: make(map[[32]byte]time.Time)}
	mux := http.NewServeMux()
	mux.HandleFunc(healthRoute, func(w http.ResponseWriter, r *http.Request) {
		if err := health(r.Context()); err != nil {
			fail(w, http.StatusServiceUnavailable, unhealthyMessage)
			return
		}
		reply(w, http.StatusOK, map[string]string{statusField: healthyStatus})
	})
	mux.HandleFunc(loginRoute, s.login)
	mux.HandleFunc(logoutRoute, s.logout)
	mux.HandleFunc(sessionRoute, func(w http.ResponseWriter, r *http.Request) {
		reply(w, http.StatusOK, map[string]bool{serverAuthenticatedField: s.authenticated(r)})
	})
	mux.HandleFunc(listImagesRoute, s.listImages)
	mux.HandleFunc(getImageRoute, s.getImage)
	mux.HandleFunc(listPostsRoute, s.listPosts)
	mux.HandleFunc(getPostRoute, s.getPost)
	adminMux := http.NewServeMux()
	adminMux.HandleFunc(listAdminPostsRoute, s.listPosts)
	adminMux.Handle(uploadPhotoRoute, limitConcurrency(cfg.MaxUploads, http.HandlerFunc(s.uploadImage)))
	adminMux.HandleFunc(updatePhotoRoute, s.saveImage)
	adminMux.HandleFunc(movePhotoRoute, s.moveImage)
	adminMux.HandleFunc(deletePhotoRoute, s.deleteImage)
	adminMux.HandleFunc(deleteBlogImageRoute, s.deleteBlogImage)
	adminMux.Handle(uploadBlogImageRoute, limitConcurrency(cfg.MaxUploads, http.HandlerFunc(s.uploadBlogImage)))
	adminMux.HandleFunc(createPostRoute, s.savePost)
	adminMux.HandleFunc(updatePostRoute, s.savePost)
	adminMux.HandleFunc(deletePostRoute, s.deletePost)
	adminMux.HandleFunc(rootPath, unknownEndpoint)

	protectedAdmin := s.admin(adminMux)
	mux.Handle(adminAPIPath, protectedAdmin)
	mux.Handle(adminAPIPrefix, protectedAdmin)
	mux.HandleFunc(apiPrefix, unknownEndpoint)
	mux.HandleFunc(rootPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set(allowHeader, frontendAllowedMethods)
			http.Error(w, methodNotAllowedMessage, http.StatusMethodNotAllowed)
			return
		}
		s.frontend(w, r)
	})
	policy := contentSecurityPolicy(cfg.PhotoOrigin, cfg.BlogOrigin)
	protection := http.NewCrossOriginProtection()
	handler := limitConcurrency(cfg.MaxRequests, protection.Handler(mux))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentSecurityPolicyHeader, policy)
		w.Header().Set(contentTypeOptionsHeader, noSniff)
		w.Header().Set(referrerPolicyHeader, strictOriginPolicy)
		w.Header().Set(frameOptionsHeader, denyFraming)
		if strings.HasPrefix(r.URL.Path, apiPrefix) {
			w.Header().Set(serverCacheControlHeader, noStore)
		}
		handler.ServeHTTP(w, r)
	})
}

func unknownEndpoint(w http.ResponseWriter, _ *http.Request) {
	fail(w, http.StatusNotFound, unknownEndpointMessage)
}

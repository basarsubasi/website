package httpadapter

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"time"
	"uuid"
)

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
			fail(w, http.StatusUnauthorized, "Please sign in")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Password == "" {
		fail(w, http.StatusServiceUnavailable, "Admin login is disabled; configure ADMIN_PASSWORD")
		return
	}
	now := time.Now()
	limited := !s.logins.allow(s.clientAddress(r), now)
	if limited {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "Too many login attempts. Try again in a minute.")
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
		fail(w, http.StatusUnauthorized, "Invalid password")
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
	reply(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("admin_session"); err == nil {
		s.mu.Lock()
		delete(s.sessions, sha256.Sum256([]byte(cookie.Value)))
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "admin_session", Path: "/api", HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	reply(w, http.StatusNoContent, nil)
}

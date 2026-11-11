package httpadapter

import "net/http"

const (
	DefaultMaxRequests = 32
	DefaultMaxUploads  = 2
)

// limitConcurrency rejects excess work instead of accumulating waiting handlers.
func limitConcurrency(max int, next http.Handler) http.Handler {
	slots := make(chan struct{}, max)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Retry-After", "1")
			fail(w, http.StatusServiceUnavailable, "Server busy; please retry")
		}
	})
}

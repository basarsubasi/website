package httpadapter

import (
	"net/http"
)

const (
	concurrencyRetryAfterHeader = "Retry-After"
	busyRetrySeconds            = "1"
	serverBusyMessage           = "Server busy; please retry"
)

const (
	DefaultMaxRequests = 32
	DefaultMaxUploads  = 2
)

// limitConcurrency rejects excess work instead of accumulating waiting handlers.
func limitConcurrency(limit int, next http.Handler) http.Handler {
	slots := make(chan struct{}, limit)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
			next.ServeHTTP(w, r)
		default:
			w.Header().Set(concurrencyRetryAfterHeader, busyRetrySeconds)
			fail(w, http.StatusServiceUnavailable, serverBusyMessage)
		}
	})
}

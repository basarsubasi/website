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

// A concurrencyLimit shares slots across all handlers wrapped by the same limit.
type concurrencyLimit chan struct{}

func (slots concurrencyLimit) wrap(next http.Handler) http.Handler {
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

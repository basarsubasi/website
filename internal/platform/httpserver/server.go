// Package httpserver bounds inbound connections and owns HTTP server shutdown.
package httpserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

const (
	requestTimeout  = 2 * time.Minute
	shutdownTimeout = 10 * time.Second
)

func New(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr: address,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
			defer cancel()
			handler.ServeHTTP(w, r.WithContext(ctx))
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       requestTimeout,
		WriteTimeout:      requestTimeout,
		IdleTimeout:       time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
}

// Serve owns the listener and waits for shutdown on cancellation or an accept error.
func Serve(ctx context.Context, server *http.Server, listener net.Listener, maxConnections int) error {
	ctx, stop := context.WithCancel(ctx)
	stopped := make(chan error, 1)
	go func() {
		<-ctx.Done()
		deadline, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		err := server.Shutdown(deadline)
		if err != nil {
			_ = server.Close()
		}
		stopped <- err
	}()
	err := server.Serve(LimitConnections(listener, maxConnections))
	stop()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return errors.Join(err, <-stopped)
}

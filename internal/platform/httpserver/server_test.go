package httpserver

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

const (
	requestHeaders  = "GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
	cancelServer    = "cancel server"
	closeListener   = "listener failure"
	missingDeadline = "handler received no request deadline"
	earlyShutdown   = "server returned while a request was still active"
)

func TestServeDrainsActiveRequests(t *testing.T) {
	for _, cause := range []string{cancelServer, closeListener} {
		t.Run(cause, func(t *testing.T) {
			listener, err := net.Listen(listenerTestTcpNetwork, ephemeralAddress)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			entered := make(chan struct{})
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			server := New(listener.Addr().String(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > requestTimeout {
					t.Error(missingDeadline)
				}
				close(entered)
				<-release
				w.WriteHeader(http.StatusNoContent)
			}))
			shuttingDown := make(chan struct{})
			server.RegisterOnShutdown(func() { close(shuttingDown) })
			result := make(chan error, 1)
			go func() { result <- Serve(ctx, server, listener, 2) }()
			conn, err := net.DialTimeout(listenerTestTcpNetwork, listener.Addr().String(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if _, err := io.WriteString(conn, requestHeaders); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			var want error
			if cause == closeListener {
				listener.Close()
				want = net.ErrClosed
			} else {
				cancel()
			}
			select {
			case <-shuttingDown:
			case <-time.After(3 * time.Second):
				t.Fatal(blockedShutdownFailure)
			}
			select {
			case <-result:
				t.Fatal(earlyShutdown)
			default:
			}
			unblock()
			select {
			case err := <-result:
				if !errors.Is(err, want) {
					t.Fatalf(shutdownErrorFormat, err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal(blockedShutdownFailure)
			}
		})
	}
}

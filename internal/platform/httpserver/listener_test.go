package httpserver

import (
	"errors"
	"net"
	"testing"
	"time"
)

const (
	listenerTestTcpNetwork = "tcp"
	ephemeralAddress       = "127.0.0.1:0"
	connectionLimitFailure = "connection limit exceeded"
	capacityReleaseFailure = "closed connection did not release capacity"
	shutdownErrorFormat    = "shutdown error: %v"
	blockedShutdownFailure = "shutdown blocked at connection limit"
	closedListenerFailure  = "closed listener accepted"
	leakedCapacityFailure  = "failed Accept leaked capacity"
)

func TestConnectionLimitAndShutdown(t *testing.T) {
	raw, err := net.Listen(listenerTestTcpNetwork, ephemeralAddress)
	if err != nil {
		t.Fatal(err)
	}
	listener := LimitConnections(raw, 1)
	defer listener.Close()
	dial := func() net.Conn {
		t.Helper()
		conn, err := net.DialTimeout(listenerTestTcpNetwork, listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		return conn
	}
	dial()
	first, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	dial()
	accepted := make(chan net.Conn, 1)
	failures := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			failures <- err
			return
		}
		accepted <- conn
	}()
	select {
	case conn := <-accepted:
		conn.Close()
		t.Fatal(connectionLimitFailure)
	case err := <-failures:
		t.Fatal(err)
	case <-time.After(50 * time.Millisecond):
	}
	first.Close()
	first.Close() // Must not release another connection's slot.
	var second net.Conn
	select {
	case second = <-accepted:
	case err := <-failures:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal(capacityReleaseFailure)
	}
	defer second.Close()
	go func() { _, err := listener.Accept(); failures <- err }()
	listener.Close()
	select {
	case err := <-failures:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf(shutdownErrorFormat, err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal(blockedShutdownFailure)
	}
}

func TestConnectionAcceptErrorReleasesCapacity(t *testing.T) {
	raw, err := net.Listen(listenerTestTcpNetwork, ephemeralAddress)
	if err != nil {
		t.Fatal(err)
	}
	listener := LimitConnections(raw, 1)
	defer listener.Close()
	raw.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 2 {
			if _, err := listener.Accept(); err == nil {
				t.Error(closedListenerFailure)
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal(leakedCapacityFailure)
	}
}

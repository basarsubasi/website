package application

import (
	"net"
	"sync"
)

const defaultMaxConnections = 128

// limitedListener stops accepting when all connection slots are occupied.
// Further connections wait in the OS backlog, not in application goroutines.
type limitedListener struct {
	net.Listener
	slots chan struct{}
	done  chan struct{}
	once  sync.Once
	err   error
}

func limitConnections(listener net.Listener, max int) net.Listener {
	if max <= 0 {
		max = defaultMaxConnections
	}
	return &limitedListener{Listener: listener, slots: make(chan struct{}, max), done: make(chan struct{})}
}

func (l *limitedListener) Accept() (net.Conn, error) {
	select {
	case l.slots <- struct{}{}:
	case <-l.done:
		return nil, net.ErrClosed
	}
	conn, err := l.Listener.Accept()
	if err != nil {
		<-l.slots
		return nil, err
	}
	return &limitedConn{Conn: conn, release: func() { <-l.slots }}, nil
}

func (l *limitedListener) Close() error {
	l.once.Do(func() {
		close(l.done)
		l.err = l.Listener.Close()
	})
	return l.err
}

type limitedConn struct {
	net.Conn
	release func()
	once    sync.Once
}

func (c *limitedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.release)
	return err
}

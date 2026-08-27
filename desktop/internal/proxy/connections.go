package proxy

import (
	"net"
	"sync"
)

// ConnectionTracker owns client-side proxy connections so a network change can
// terminate every existing tunnel before traffic resumes on the new mobile IP.
type ConnectionTracker struct {
	mu     sync.Mutex
	active map[*trackedConnection]struct{}
}

func NewConnectionTracker() *ConnectionTracker {
	return &ConnectionTracker{active: make(map[*trackedConnection]struct{})}
}

func (t *ConnectionTracker) Wrap(connection net.Conn) net.Conn {
	if t == nil || connection == nil {
		return connection
	}
	tracked := &trackedConnection{Conn: connection, owner: t}
	t.mu.Lock()
	t.active[tracked] = struct{}{}
	t.mu.Unlock()
	return tracked
}

func (t *ConnectionTracker) CloseAll() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	connections := make([]*trackedConnection, 0, len(t.active))
	for connection := range t.active {
		connections = append(connections, connection)
	}
	t.mu.Unlock()
	for _, connection := range connections {
		_ = connection.Close()
	}
	return len(connections)
}

func (t *ConnectionTracker) remove(connection *trackedConnection) {
	t.mu.Lock()
	delete(t.active, connection)
	t.mu.Unlock()
}

type trackedConnection struct {
	net.Conn
	owner *ConnectionTracker
	close sync.Once
	err   error
}

func (c *trackedConnection) Close() error {
	c.close.Do(func() {
		c.owner.remove(c)
		c.err = c.Conn.Close()
	})
	return c.err
}

package proxy

import (
	"context"
	"io"
	"net"
	"sync"
)

type ContextDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

// Relay copies bytes in both directions and preserves TCP half-close behavior.
func Relay(left, right net.Conn) {
	var wait sync.WaitGroup
	wait.Add(2)
	copyOneWay := func(destination, source net.Conn) {
		defer wait.Done()
		_, _ = io.Copy(destination, source)
		if closeWriter, ok := destination.(interface{ CloseWrite() error }); ok {
			_ = closeWriter.CloseWrite()
		}
	}
	go copyOneWay(left, right)
	go copyOneWay(right, left)
	wait.Wait()
}

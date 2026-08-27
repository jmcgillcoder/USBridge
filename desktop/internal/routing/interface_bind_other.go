//go:build !windows

package routing

import "net"

func bindDialerToInterface(_ *net.Dialer, _ int, _ bool) {}

//go:build windows

package routing

import (
	"math/bits"
	"net"
	"syscall"

	"golang.org/x/sys/windows"
)

const windowsUnicastInterfaceOption = 31

func bindDialerToInterface(dialer *net.Dialer, interfaceIndex int, ipv6 bool) {
	if dialer == nil || interfaceIndex <= 0 {
		return
	}
	dialer.Control = func(_, _ string, rawConnection syscall.RawConn) error {
		var socketError error
		if err := rawConnection.Control(func(fileDescriptor uintptr) {
			level := windows.IPPROTO_IP
			value := int(bits.ReverseBytes32(uint32(interfaceIndex)))
			if ipv6 {
				level = windows.IPPROTO_IPV6
				value = interfaceIndex
			}
			socketError = windows.SetsockoptInt(
				windows.Handle(fileDescriptor),
				level,
				windowsUnicastInterfaceOption,
				value,
			)
		}); err != nil {
			return err
		}
		return socketError
	}
}

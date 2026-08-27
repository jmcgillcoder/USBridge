//go:build !windows

package adapter

import (
	"context"
	"net"
	"strconv"
)

type genericProvider struct{}

func NewProvider() Provider {
	return genericProvider{}
}

func (genericProvider) List(context.Context) ([]Adapter, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	result := make([]Adapter, 0, len(interfaces))
	for _, networkInterface := range interfaces {
		value := Adapter{
			ID:             strconv.Itoa(networkInterface.Index),
			Name:           networkInterface.Name,
			Description:    networkInterface.Name,
			InterfaceIndex: networkInterface.Index,
			MACAddress:     networkInterface.HardwareAddr.String(),
			Status:         "Disconnected",
		}
		if networkInterface.Flags&net.FlagUp != 0 {
			value.Status = "Up"
		}
		addresses, _ := networkInterface.Addrs()
		for _, address := range addresses {
			ip, _, parseErr := net.ParseCIDR(address.String())
			if parseErr != nil || ip == nil {
				continue
			}
			if ip.To4() != nil {
				value.IPv4 = append(value.IPv4, ip.String())
			} else {
				value.IPv6 = append(value.IPv6, ip.String())
			}
		}
		result = append(result, value)
	}
	return result, nil
}

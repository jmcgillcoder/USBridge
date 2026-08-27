package adapter

import (
	"context"
	"net"
	"strings"
)

// Adapter is the subset of Windows adapter state needed by routing and UI.
type Adapter struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	InterfaceIndex int      `json:"interfaceIndex"`
	Status         string   `json:"status"`
	MACAddress     string   `json:"macAddress,omitempty"`
	LinkSpeed      uint64   `json:"linkSpeed,omitempty"`
	IPv4           []string `json:"ipv4"`
	IPv6           []string `json:"ipv6"`
	Gateways       []string `json:"gateways"`
	DNSServers     []string `json:"dnsServers,omitempty"`
	USBCandidate   bool     `json:"usbCandidate"`
	Score          int      `json:"score"`
}

func (a Adapter) IsUp() bool {
	return strings.EqualFold(a.Status, "up")
}

func (a Adapter) IPs(ipv6 bool) []net.IP {
	values := a.IPv4
	if ipv6 {
		values = a.IPv6
	}
	result := make([]net.IP, 0, len(values))
	for _, value := range values {
		if ip := net.ParseIP(stripZone(value)); ip != nil {
			result = append(result, ip)
		}
	}
	return result
}

func stripZone(value string) string {
	if index := strings.LastIndexByte(value, '%'); index >= 0 {
		return value[:index]
	}
	return value
}

type Provider interface {
	List(context.Context) ([]Adapter, error)
}

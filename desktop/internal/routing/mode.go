package routing

import (
	"fmt"
	"strings"
	"sync/atomic"
)

type IPMode uint32

const (
	IPModeAuto IPMode = iota
	IPModeIPv4
	IPModeIPv6
)

func ParseIPMode(value string) (IPMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto", "automatic":
		return IPModeAuto, nil
	case "ipv4", "v4", "4":
		return IPModeIPv4, nil
	case "ipv6", "v6", "6":
		return IPModeIPv6, nil
	default:
		return IPModeAuto, fmt.Errorf("unsupported IP mode %q", value)
	}
}

func (m IPMode) String() string {
	switch m {
	case IPModeIPv4:
		return "ipv4"
	case IPModeIPv6:
		return "ipv6"
	default:
		return "auto"
	}
}

type Policy struct {
	mode atomic.Uint32
}

func NewPolicy(mode IPMode) *Policy {
	policy := &Policy{}
	policy.SetMode(mode)
	return policy
}

func (p *Policy) Mode() IPMode {
	return IPMode(p.mode.Load())
}

func (p *Policy) SetMode(mode IPMode) {
	p.mode.Store(uint32(mode))
}

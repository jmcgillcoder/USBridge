package routing

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/usbridge/usbridge/desktop/internal/adapter"
)

type staticAdapterSource struct {
	value adapter.Adapter
}

func TestPlanDialAttemptsAutoRacesIPv6BeforeIPv4(t *testing.T) {
	selected := adapter.Adapter{
		InterfaceIndex: 17,
		IPv4:           []string{"10.179.36.10"},
		IPv6:           []string{"2607:fb90:69b:62b2::10"},
	}
	candidates := []net.IPAddr{
		{IP: net.ParseIP("198.51.100.8")},
		{IP: net.ParseIP("2001:db8::8")},
	}

	attempts := planDialAttempts(selected, candidates, "443")
	if len(attempts) != 2 {
		t.Fatalf("attempts = %+v", attempts)
	}
	if attempts[0].network != "tcp6" || attempts[0].delay != 0 {
		t.Fatalf("first attempt = %+v", attempts[0])
	}
	if attempts[1].network != "tcp4" || attempts[1].delay != autoFallbackDelay {
		t.Fatalf("fallback attempt = %+v", attempts[1])
	}
}

func TestPlanDialAttemptsSkipsUnavailableAddressFamily(t *testing.T) {
	selected := adapter.Adapter{IPv4: []string{"10.179.36.10"}}
	candidates := []net.IPAddr{
		{IP: net.ParseIP("2001:db8::8")},
		{IP: net.ParseIP("198.51.100.8")},
	}

	attempts := planDialAttempts(selected, candidates, "443")
	if len(attempts) != 1 || attempts[0].network != "tcp4" || attempts[0].delay != 0 {
		t.Fatalf("attempts = %+v", attempts)
	}
}

func TestScheduleDialAttemptsStaggersSameFamily(t *testing.T) {
	values := []dialAttempt{{network: "tcp6"}, {network: "tcp6"}}
	attempts := scheduleDialAttempts(values, autoFallbackDelay)

	if attempts[0].delay != 250*time.Millisecond || attempts[1].delay != 500*time.Millisecond {
		t.Fatalf("attempt delays = %v, %v", attempts[0].delay, attempts[1].delay)
	}
}

func (source staticAdapterSource) Selected() (adapter.Adapter, bool) {
	return source.value, true
}

func TestBoundDialerFailsClosedWhenUpstreamGateRejects(t *testing.T) {
	dialer := NewBoundDialer(
		staticAdapterSource{value: adapter.Adapter{
			Name:   "USB",
			Status: "Up",
			IPv4:   []string{"10.0.0.2"},
		}},
		nil,
	)
	dialer.Gate = func() error { return ErrNoCellularUpstream }

	_, err := dialer.DialContext(context.Background(), "tcp", "example.com:80")
	if !errors.Is(err, ErrNoCellularUpstream) {
		t.Fatalf("error = %v", err)
	}
}

func TestSourceAddressesKeepEveryUsableAddress(t *testing.T) {
	value := adapter.Adapter{
		IPv6: []string{
			"fe80::1%12",
			"2001:db8::1",
			"2001:db8::2",
		},
	}

	global := sourceAddresses(value, true, false)
	if len(global) != 2 || global[0].String() != "2001:db8::1" || global[1].String() != "2001:db8::2" {
		t.Fatalf("global sources = %v", global)
	}
	withLinkLocal := sourceAddresses(value, true, true)
	if len(withLinkLocal) != 3 {
		t.Fatalf("sources with link-local = %v", withLinkLocal)
	}
}

func TestDiscoverNAT64PrefixFromTMobileDNS64(t *testing.T) {
	prefix, ok := discoverNAT64Prefix([]net.IP{
		net.ParseIP("2607:7700:0:2:0:2:c000:ab"),
		net.ParseIP("2607:7700:0:2:0:2:c000:aa"),
	})
	if !ok {
		t.Fatal("NAT64 prefix was not discovered")
	}
	if prefix.bits != 96 || prefix.network.String() != "2607:7700:0:2:0:2::" {
		t.Fatalf("prefix = %s/%d", prefix.network, prefix.bits)
	}
	translated := synthesizeNAT64Address(prefix, net.ParseIP("1.1.1.1"))
	if translated.String() != "2607:7700:0:2:0:2:101:101" {
		t.Fatalf("translated address = %s", translated)
	}
}

func TestNAT64SynthesisSupportsEveryRFC6052PrefixLength(t *testing.T) {
	tests := []struct {
		prefix   string
		bits     int
		expected string
	}{
		{prefix: "2001:db8::", bits: 32, expected: "2001:db8:c000:221::"},
		{prefix: "2001:db8:1200::", bits: 40, expected: "2001:db8:12c0:2:21::"},
		{prefix: "2001:db8:122::", bits: 48, expected: "2001:db8:122:c000:2:2100::"},
		{prefix: "2001:db8:122:3400::", bits: 56, expected: "2001:db8:122:34c0:0:221::"},
		{prefix: "2001:db8:122:344::", bits: 64, expected: "2001:db8:122:344:c0:2:2100:0"},
		{
			prefix:   "2001:db8:122:344:5566:7788::",
			bits:     96,
			expected: "2001:db8:122:344:5566:7788:c000:221",
		},
	}

	for _, test := range tests {
		t.Run(test.prefix, func(t *testing.T) {
			prefix := nat64Prefix{network: net.ParseIP(test.prefix), bits: test.bits}
			translated := synthesizeNAT64Address(prefix, net.ParseIP("192.0.2.33"))
			if translated == nil || translated.String() != test.expected {
				t.Fatalf("translated address = %v, want %s", translated, test.expected)
			}

			discoveryAddress := synthesizeNAT64Address(prefix, net.ParseIP("192.0.0.170"))
			discovered, ok := discoverNAT64Prefix([]net.IP{discoveryAddress})
			if !ok || discovered.bits != test.bits || !discovered.network.Equal(prefix.network) {
				t.Fatalf(
					"discovered prefix = %s/%d, want %s/%d",
					discovered.network,
					discovered.bits,
					prefix.network,
					test.bits,
				)
			}
		})
	}
}

func TestDiscoverNAT64PrefixRejectsAmbiguousAddress(t *testing.T) {
	_, ok := discoverNAT64Prefix([]net.IP{
		net.ParseIP("2001:db8:c000:aa:0:0:c000:aa"),
	})
	if ok {
		t.Fatal("ambiguous RFC 7050 address was accepted")
	}
}

func TestResolveIPv4ModeUsesNAT64TransportWithLocalIPv4Present(t *testing.T) {
	selected := adapter.Adapter{
		Name:           "USB",
		InterfaceIndex: 49,
		IPv4:           []string{"10.179.36.25"},
		IPv6:           []string{"2607:fb90:f06d:a61b::25"},
		DNSServers:     []string{"2607:fb90::1"},
		Gateways:       []string{"fe80::1"},
	}
	dialer := NewBoundDialer(staticAdapterSource{value: selected}, NewPolicy(IPModeIPv4))
	dialer.nat64Cache = nat64CacheEntry{
		key: nat64NetworkKey(selected),
		prefix: nat64Prefix{
			network: net.ParseIP("2607:7700:0:2:0:2::"),
			bits:    96,
		},
		expiresAt: time.Now().Add(time.Minute),
	}

	candidates, err := dialer.resolveLiteralIP(
		context.Background(),
		selected,
		net.ParseIP("1.1.1.1"),
		IPModeIPv4,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 ||
		candidates[0].IP.String() != "2607:7700:0:2:0:2:101:101" ||
		candidates[1].IP.String() != "1.1.1.1" {
		t.Fatalf("candidates = %v", candidates)
	}

	attempts := planDialAttempts(selected, candidates, "443")
	if len(attempts) != 2 || attempts[0].network != "tcp6" || attempts[1].network != "tcp4" {
		t.Fatalf("attempts = %+v", attempts)
	}
}

func TestResolveIPv4CandidatesTranslatesEveryAddressOnIPv6OnlyTransport(t *testing.T) {
	selected := adapter.Adapter{
		Name:           "USB",
		InterfaceIndex: 49,
		IPv6:           []string{"2607:fb90:f06d:a61b::25"},
	}
	dialer := NewBoundDialer(staticAdapterSource{value: selected}, NewPolicy(IPModeIPv4))
	dialer.nat64Cache = nat64CacheEntry{
		key: nat64NetworkKey(selected),
		prefix: nat64Prefix{
			network: net.ParseIP("64:ff9b::"),
			bits:    96,
		},
		expiresAt: time.Now().Add(time.Minute),
	}

	candidates, err := dialer.resolveIPv4Candidates(context.Background(), selected, []net.IPAddr{
		{IP: net.ParseIP("192.0.2.10")},
		{IP: net.ParseIP("198.51.100.20")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 ||
		candidates[0].IP.String() != "64:ff9b::c000:20a" ||
		candidates[1].IP.String() != "64:ff9b::c633:6414" {
		t.Fatalf("candidates = %v", candidates)
	}
}

func TestResolveLiteralRejectsTargetFamilyMismatch(t *testing.T) {
	selected := adapter.Adapter{}
	dialer := NewBoundDialer(staticAdapterSource{value: selected}, NewPolicy(IPModeAuto))

	if _, err := dialer.resolveLiteralIP(
		context.Background(), selected, net.ParseIP("2001:db8::1"), IPModeIPv4,
	); err == nil {
		t.Fatal("IPv6 literal was accepted in IPv4 mode")
	}
	if _, err := dialer.resolveLiteralIP(
		context.Background(), selected, net.ParseIP("192.0.2.1"), IPModeIPv6,
	); err == nil {
		t.Fatal("IPv4 literal was accepted in IPv6 mode")
	}
}

package routing

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/usbridge/usbridge/desktop/internal/adapter"
)

var ErrNoUSBAdapter = errors.New("no active USB tethering adapter is selected")
var ErrNoCellularUpstream = errors.New("phone USB tethering is not using a cellular upstream")

const autoFallbackDelay = 250 * time.Millisecond
const nat64CacheTTL = 5 * time.Minute

var nat64PrefixLengths = []int{96, 64, 56, 48, 40, 32}

type AdapterSource interface {
	Selected() (adapter.Adapter, bool)
}

type BoundDialer struct {
	Adapters  AdapterSource
	Policy    *Policy
	Resolver  *net.Resolver
	Gate      func() error
	Timeout   time.Duration
	KeepAlive time.Duration

	nat64Mu    sync.RWMutex
	nat64Cache nat64CacheEntry
}

type nat64Prefix struct {
	network net.IP
	bits    int
}

type nat64CacheEntry struct {
	key       string
	prefix    nat64Prefix
	expiresAt time.Time
}

type dialAttempt struct {
	network  string
	target   string
	sourceIP net.IP
	ipv6     bool
	delay    time.Duration
}

type dialResult struct {
	connection net.Conn
	err        error
}

func NewBoundDialer(adapters AdapterSource, policy *Policy) *BoundDialer {
	return &BoundDialer{
		Adapters:  adapters,
		Policy:    policy,
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}
}

func (d *BoundDialer) DialContext(ctx context.Context, _ string, address string) (net.Conn, error) {
	selected, ok := d.Adapters.Selected()
	if !ok || !selected.IsUp() {
		return nil, ErrNoUSBAdapter
	}
	if d.Gate != nil {
		if gateErr := d.Gate(); gateErr != nil {
			return nil, gateErr
		}
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy destination %q: %w", address, err)
	}
	mode := IPModeAuto
	if d.Policy != nil {
		mode = d.Policy.Mode()
	}
	candidates, err := d.resolve(ctx, selected, host, mode)
	if err != nil {
		return nil, err
	}

	attempts := planDialAttempts(selected, candidates, port)
	if len(attempts) == 0 {
		return nil, fmt.Errorf("selected adapter %q has no usable transport for %s targets", selected.Name, mode)
	}
	return d.raceDialAttempts(ctx, selected.InterfaceIndex, attempts)
}

func planDialAttempts(
	selected adapter.Adapter,
	candidates []net.IPAddr,
	port string,
) []dialAttempt {
	var ipv4Attempts []dialAttempt
	var ipv6Attempts []dialAttempt
	for _, candidate := range candidates {
		isIPv6 := candidate.IP.To4() == nil
		network := "tcp4"
		if isIPv6 {
			network = "tcp6"
		}
		target := net.JoinHostPort(candidate.IP.String(), port)
		for _, sourceIP := range sourceAddresses(selected, isIPv6, false) {
			attempt := dialAttempt{
				network:  network,
				target:   target,
				sourceIP: sourceIP,
				ipv6:     isIPv6,
			}
			if isIPv6 {
				ipv6Attempts = append(ipv6Attempts, attempt)
			} else {
				ipv4Attempts = append(ipv4Attempts, attempt)
			}
		}
	}

	if len(ipv6Attempts) == 0 {
		return scheduleDialAttempts(ipv4Attempts, 0)
	}
	if len(ipv4Attempts) == 0 {
		return scheduleDialAttempts(ipv6Attempts, 0)
	}

	result := scheduleDialAttempts(ipv6Attempts, 0)
	result = append(result, scheduleDialAttempts(ipv4Attempts, autoFallbackDelay)...)
	return result
}

func scheduleDialAttempts(values []dialAttempt, initialDelay time.Duration) []dialAttempt {
	result := make([]dialAttempt, len(values))
	copy(result, values)
	for index := range result {
		result[index].delay = initialDelay + time.Duration(index)*autoFallbackDelay
	}
	return result
}

func (d *BoundDialer) raceDialAttempts(
	ctx context.Context,
	interfaceIndex int,
	attempts []dialAttempt,
) (net.Conn, error) {
	raceContext, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan dialResult)

	for _, attempt := range attempts {
		attempt := attempt
		go func() {
			if attempt.delay > 0 {
				timer := time.NewTimer(attempt.delay)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-raceContext.Done():
					return
				}
			}

			dialer := net.Dialer{
				Timeout:   d.Timeout,
				KeepAlive: d.KeepAlive,
				LocalAddr: &net.TCPAddr{IP: attempt.sourceIP},
			}
			bindDialerToInterface(&dialer, interfaceIndex, attempt.ipv6)
			connection, err := dialer.DialContext(raceContext, attempt.network, attempt.target)
			if err != nil {
				err = fmt.Errorf("%s via %s: %w", attempt.target, attempt.sourceIP, err)
			}
			select {
			case results <- dialResult{connection: connection, err: err}:
			case <-raceContext.Done():
				if connection != nil {
					_ = connection.Close()
				}
			}
		}()
	}

	failures := make([]error, 0, len(attempts))
	for range attempts {
		select {
		case result := <-results:
			if result.err == nil {
				cancel()
				return result.connection, nil
			}
			failures = append(failures, result.err)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, errors.Join(failures...)
}

func (d *BoundDialer) resolve(
	ctx context.Context,
	selected adapter.Adapter,
	host string,
	mode IPMode,
) ([]net.IPAddr, error) {
	plainHost := host
	if index := strings.LastIndexByte(plainHost, '%'); index >= 0 {
		plainHost = plainHost[:index]
	}
	if ip := net.ParseIP(plainHost); ip != nil {
		return d.resolveLiteralIP(ctx, selected, ip, mode)
	}
	resolver := d.Resolver
	if resolver == nil {
		resolver = d.resolverForAdapter(selected)
	}
	lookupNetwork := "ip"
	if mode == IPModeIPv4 {
		lookupNetwork = "ip4"
	} else if mode == IPModeIPv6 {
		lookupNetwork = "ip6"
	}
	resolved, err := resolver.LookupIP(ctx, lookupNetwork, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}
	values := make([]net.IPAddr, 0, len(resolved))
	for _, ip := range resolved {
		values = append(values, net.IPAddr{IP: ip})
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("resolve %q: no addresses", host)
	}
	if mode == IPModeIPv4 {
		return d.resolveIPv4Candidates(ctx, selected, values)
	}
	return values, nil
}

func (d *BoundDialer) resolveLiteralIP(
	ctx context.Context,
	selected adapter.Adapter,
	ip net.IP,
	mode IPMode,
) ([]net.IPAddr, error) {
	ipv4 := ip.To4()
	if ipv4 == nil {
		if mode == IPModeIPv4 {
			return nil, fmt.Errorf("IPv6 destination %s is not available in IPv4 mode", ip)
		}
		return []net.IPAddr{{IP: ip}}, nil
	}
	if mode == IPModeIPv6 {
		return nil, fmt.Errorf("IPv4 destination %s is not available in IPv6 mode", ipv4)
	}
	return d.resolveIPv4Candidates(ctx, selected, []net.IPAddr{{IP: ipv4}})
}

func (d *BoundDialer) resolveIPv4Candidates(
	ctx context.Context,
	selected adapter.Adapter,
	values []net.IPAddr,
) ([]net.IPAddr, error) {
	hasIPv4Transport := len(sourceAddresses(selected, false, false)) > 0
	hasIPv6Transport := len(sourceAddresses(selected, true, false)) > 0
	if !hasIPv6Transport {
		return values, nil
	}

	prefix, prefixErr := d.nat64Prefix(ctx, selected)
	if prefixErr == nil {
		translated := make([]net.IPAddr, 0, len(values))
		for _, value := range values {
			ipv4 := value.IP.To4()
			if ipv4 == nil {
				continue
			}
			address := synthesizeNAT64Address(prefix, ipv4)
			if address != nil {
				translated = append(translated, net.IPAddr{IP: address})
			}
		}
		if len(translated) > 0 {
			if hasIPv4Transport {
				translated = append(translated, values...)
			}
			return translated, nil
		}
		prefixErr = errors.New("discovered prefix could not synthesize an IPv6 address")
	}
	if hasIPv4Transport {
		return values, nil
	}
	return nil, fmt.Errorf("discover NAT64 prefix for IPv4 destination: %w", prefixErr)
}

func (d *BoundDialer) nat64Prefix(
	ctx context.Context,
	selected adapter.Adapter,
) (nat64Prefix, error) {
	cacheKey := nat64NetworkKey(selected)
	now := time.Now()
	d.nat64Mu.RLock()
	cached := d.nat64Cache
	d.nat64Mu.RUnlock()
	if cached.key == cacheKey && now.Before(cached.expiresAt) && cached.prefix.network != nil {
		return cached.prefix, nil
	}

	resolver := d.Resolver
	if resolver == nil {
		resolver = d.resolverForAdapter(selected)
	}
	addresses, err := resolver.LookupIP(ctx, "ip6", "ipv4only.arpa.")
	if err != nil {
		return nat64Prefix{}, err
	}
	prefix, ok := discoverNAT64Prefix(addresses)
	if !ok {
		return nat64Prefix{}, errors.New("ipv4only.arpa did not return an RFC 7050 address")
	}

	d.nat64Mu.Lock()
	d.nat64Cache = nat64CacheEntry{
		key:       cacheKey,
		prefix:    prefix,
		expiresAt: now.Add(nat64CacheTTL),
	}
	d.nat64Mu.Unlock()
	return prefix, nil
}

func nat64NetworkKey(selected adapter.Adapter) string {
	return fmt.Sprintf(
		"%d|%s|%s|%s|%s",
		selected.InterfaceIndex,
		selected.Name,
		strings.Join(selected.IPv6, ","),
		strings.Join(selected.DNSServers, ","),
		strings.Join(selected.Gateways, ","),
	)
}

func discoverNAT64Prefix(addresses []net.IP) (nat64Prefix, bool) {
	for _, address := range addresses {
		ipv6 := address.To16()
		if ipv6 == nil || address.To4() != nil {
			continue
		}
		var match *nat64Prefix
		ambiguous := false
		for _, bits := range nat64PrefixLengths {
			embedded, ok := extractEmbeddedIPv4(ipv6, bits)
			if !ok || embedded[0] != 192 || embedded[1] != 0 || embedded[2] != 0 ||
				(embedded[3] != 170 && embedded[3] != 171) {
				continue
			}
			mask := net.CIDRMask(bits, net.IPv6len*8)
			candidate := nat64Prefix{
				network: append(net.IP(nil), ipv6.Mask(mask)...),
				bits:    bits,
			}
			if match != nil {
				ambiguous = true
				break
			}
			match = &candidate
		}
		if !ambiguous && match != nil {
			return *match, true
		}
	}
	return nat64Prefix{}, false
}

func extractEmbeddedIPv4(ipv6 net.IP, prefixBits int) ([net.IPv4len]byte, bool) {
	var result [net.IPv4len]byte
	if len(ipv6) != net.IPv6len {
		return result, false
	}
	if prefixBits == 96 {
		copy(result[:], ipv6[12:16])
		return result, true
	}
	if prefixBits < 32 || prefixBits > 64 || prefixBits%8 != 0 || ipv6[8] != 0 {
		return result, false
	}
	beforeUOctet := (64 - prefixBits) / 8
	start := prefixBits / 8
	copy(result[:beforeUOctet], ipv6[start:8])
	copy(result[beforeUOctet:], ipv6[9:9+net.IPv4len-beforeUOctet])
	return result, true
}

func synthesizeNAT64Address(prefix nat64Prefix, ipv4 net.IP) net.IP {
	value := ipv4.To4()
	if value == nil || len(prefix.network) != net.IPv6len {
		return nil
	}
	result := append(net.IP(nil), prefix.network...)
	if prefix.bits == 96 {
		copy(result[12:16], value)
		return result
	}
	if prefix.bits < 32 || prefix.bits > 64 || prefix.bits%8 != 0 {
		return nil
	}
	beforeUOctet := (64 - prefix.bits) / 8
	start := prefix.bits / 8
	copy(result[start:8], value[:beforeUOctet])
	result[8] = 0
	copy(result[9:9+net.IPv4len-beforeUOctet], value[beforeUOctet:])
	return result
}

func (d *BoundDialer) resolverForAdapter(selected adapter.Adapter) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return d.dialDNS(ctx, selected, network, address)
		},
	}
}

func (d *BoundDialer) dialDNS(
	ctx context.Context,
	selected adapter.Adapter,
	network string,
	address string,
) (net.Conn, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		port = "53"
	}
	servers := append([]string(nil), selected.DNSServers...)
	servers = append(servers, selected.Gateways...)
	var failures []error
	seen := make(map[string]struct{})
	for _, server := range servers {
		plainServer := server
		if index := strings.LastIndexByte(plainServer, '%'); index >= 0 {
			plainServer = plainServer[:index]
		}
		ip := net.ParseIP(plainServer)
		if ip == nil || ip.IsUnspecified() || ip.IsLoopback() {
			continue
		}
		key := ip.String()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		isIPv6 := ip.To4() == nil
		sources := sourceAddresses(selected, isIPv6, ip.IsLinkLocalUnicast())
		for _, sourceIP := range sources {
			var localAddress net.Addr
			if strings.HasPrefix(network, "tcp") {
				localAddress = &net.TCPAddr{IP: sourceIP, Zone: zoneFor(sourceIP, selected.Name)}
			} else {
				localAddress = &net.UDPAddr{IP: sourceIP, Zone: zoneFor(sourceIP, selected.Name)}
			}
			targetHost := ip.String()
			if ip.IsLinkLocalUnicast() {
				targetHost += "%" + selected.Name
			}
			target := net.JoinHostPort(targetHost, port)
			dialNetwork := "udp4"
			if strings.HasPrefix(network, "tcp") {
				dialNetwork = "tcp4"
			}
			if isIPv6 {
				dialNetwork = strings.TrimSuffix(dialNetwork, "4") + "6"
			}
			dialer := net.Dialer{Timeout: dnsTimeout(d.Timeout), LocalAddr: localAddress}
			bindDialerToInterface(&dialer, selected.InterfaceIndex, isIPv6)
			connection, dialErr := dialer.DialContext(ctx, dialNetwork, target)
			if dialErr == nil {
				return connection, nil
			}
			failures = append(failures, fmt.Errorf("DNS %s via %s: %w", target, sourceIP, dialErr))
		}
	}
	if len(failures) == 0 {
		return nil, fmt.Errorf("selected adapter %q has no usable DNS server", selected.Name)
	}
	return nil, errors.Join(failures...)
}

func sourceAddresses(value adapter.Adapter, ipv6 bool, allowLinkLocal bool) []net.IP {
	result := make([]net.IP, 0)
	for _, ip := range value.IPs(ipv6) {
		if ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() ||
			(ip.IsLinkLocalUnicast() && !allowLinkLocal) {
			continue
		}
		result = append(result, ip)
	}
	return result
}

func zoneFor(ip net.IP, interfaceName string) string {
	if ip.IsLinkLocalUnicast() {
		return interfaceName
	}
	return ""
}

func dnsTimeout(configured time.Duration) time.Duration {
	const maximum = 5 * time.Second
	if configured > 0 && configured < maximum {
		return configured
	}
	return maximum
}

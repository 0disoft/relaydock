package ssrf

/* llmnav/1 module
id=relaydock.security.ssrf
role=Resolve provider and MCP destination hosts once and reject credentials, unsafe schemes, ports, and non-public addresses before dialing.
owns=outbound URL policy|DNS address validation|SSRF address allowlist
excludes=HTTP request execution|TLS certificate validation
search=validate provider URL|prevent SSRF|resolve safe dial address
invariant=Callers dial one of the already validated resolved addresses instead of repeating DNS lookup.
stability=contract
*/

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
)

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}
type Validator struct {
	AllowedSchemes []string
	AllowedPorts   []int
	Resolver       Resolver
	AllowPrivate   bool
}

func (v Validator) Validate(ctx context.Context, target *url.URL) error {
	if target == nil {
		return fmt.Errorf("%w: URL", core.ErrInvalidArgument)
	}
	if target.User != nil {
		return fmt.Errorf("%w: URL userinfo is forbidden", core.ErrForbidden)
	}
	scheme := strings.ToLower(target.Scheme)
	allowed := v.AllowedSchemes
	if len(allowed) == 0 {
		allowed = []string{"https"}
	}
	if !containsString(allowed, scheme) {
		return fmt.Errorf("%w: scheme %q", core.ErrForbidden, scheme)
	}
	host := strings.TrimSpace(target.Hostname())
	port := target.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil {
			return core.ErrInvalidArgument
		}
		if len(v.AllowedPorts) > 0 && !containsInt(v.AllowedPorts, n) {
			return fmt.Errorf("%w: port %d", core.ErrForbidden, n)
		}
	}
	_, err := v.ResolveAndValidate(ctx, host)
	return err
}

// ResolveAndValidate returns only addresses that satisfy the validator policy.
// Callers that open a connection should dial one of these returned addresses so
// validation and connection do not perform separate DNS lookups.
func (v Validator) ResolveAndValidate(ctx context.Context, host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return nil, fmt.Errorf("%w: URL host", core.ErrInvalidArgument)
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return nil, fmt.Errorf("%w: localhost", core.ErrForbidden)
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		addr = addr.Unmap()
		if err := v.validateAddr(addr); err != nil {
			return nil, err
		}
		return []netip.Addr{addr}, nil
	}
	resolver := v.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, core.ErrNotFound
	}
	resolved := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		addr, ok := netip.AddrFromSlice(address.IP)
		if !ok {
			return nil, fmt.Errorf("%w: unresolved address", core.ErrForbidden)
		}
		addr = addr.Unmap()
		if err := v.validateAddr(addr); err != nil {
			return nil, err
		}
		resolved = append(resolved, addr)
	}
	return resolved, nil
}
func (v Validator) validateAddr(addr netip.Addr) error {
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsMulticast() {
		return fmt.Errorf("%w: non-public address %s", core.ErrForbidden, addr)
	}
	if v.AllowPrivate && (addr.IsPrivate() || addr.IsLoopback()) {
		return nil
	}
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return fmt.Errorf("%w: non-public address %s", core.ErrForbidden, addr)
	}
	blocked := []netip.Prefix{netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("fc00::/7"), netip.MustParsePrefix("fe80::/10")}
	for _, prefix := range blocked {
		if prefix.Contains(addr) {
			return fmt.Errorf("%w: blocked address %s", core.ErrForbidden, addr)
		}
	}
	return nil
}
func containsString(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}
func containsInt(values []int, want int) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

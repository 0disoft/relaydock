package ssrf

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/your-org/ai-runtime-gateway/internal/core"
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
	if host == "" {
		return fmt.Errorf("%w: URL host", core.ErrInvalidArgument)
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return fmt.Errorf("%w: localhost", core.ErrForbidden)
	}
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
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return v.validateAddr(addr)
	}
	resolver := v.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return err
	}
	if len(addresses) == 0 {
		return core.ErrNotFound
	}
	for _, address := range addresses {
		addr, ok := netip.AddrFromSlice(address.IP)
		if !ok {
			return fmt.Errorf("%w: unresolved address", core.ErrForbidden)
		}
		if err := v.validateAddr(addr.Unmap()); err != nil {
			return err
		}
	}
	return nil
}
func (v Validator) validateAddr(addr netip.Addr) error {
	if v.AllowPrivate {
		return nil
	}
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() {
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

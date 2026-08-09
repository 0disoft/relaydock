package serverutil

import (
	"fmt"
	"net"
	"strings"
)

func IsLoopbackAddress(address string) (bool, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return false, err
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true, nil
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback(), nil
}

func RequireAuthenticationOutsideLoopback(address, token string) error {
	loopback, err := IsLoopbackAddress(address)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", address, err)
	}
	if !loopback && strings.TrimSpace(token) == "" {
		return fmt.Errorf("a bearer token is required when binding %s outside loopback", address)
	}
	return nil
}

//go:build !windows

package localipc

import (
	"context"
	"net"
	"os"
	"path/filepath"
)

func dialEndpoint(ctx context.Context, endpoint string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", endpoint)
}
func listenEndpoint(endpoint string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(endpoint), 0o700); err != nil {
		return nil, err
	}
	_ = os.Remove(endpoint)
	listener, err := net.Listen("unix", endpoint)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(endpoint, 0o600); err != nil {
		_ = listener.Close()
		return nil, err
	}
	return listener, nil
}
func cleanupEndpoint(endpoint string) { _ = os.Remove(endpoint) }

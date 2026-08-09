//go:build windows

package localipc

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

func dialEndpoint(ctx context.Context, endpoint string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, endpoint)
}
func listenEndpoint(endpoint string) (net.Listener, error) {
	return winio.ListenPipe(endpoint, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;OW)"})
}
func cleanupEndpoint(string) {}

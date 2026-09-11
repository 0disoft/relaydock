package desktopwails

import (
	"net"
	"strings"
	"testing"

	"github.com/0disoft/relaydock/internal/credentials"
)

func TestGatewayOccupiedPortReportsRecoveryAndCanRetry(t *testing.T) {
	clearProviderCredentialEnvironment(t)
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = occupied.Close() })
	port := occupied.Addr().(*net.TCPAddr).Port
	service := NewRuntimeServiceWithCredentialStore(nil, credentials.NewMemoryStore(), nil)
	t.Cleanup(func() { _ = service.StopLocalGateway() })
	err = service.StartLocalGateway(port)
	if err == nil || !strings.Contains(err.Error(), occupied.Addr().String()) || !strings.Contains(err.Error(), "Settings") {
		t.Fatalf("expected actionable bind failure, got %v", err)
	}
	status := service.Status()
	if status.GatewayStarting || status.GatewayReady || status.GatewayAddress != "" || status.LastError != err.Error() {
		t.Fatalf("failed start left incorrect status: %#v", status)
	}
	if err := occupied.Close(); err != nil {
		t.Fatal(err)
	}
	if err := service.StartLocalGateway(port); err != nil {
		t.Fatalf("retry after releasing port: %v", err)
	}
	status = service.Status()
	if !status.GatewayReady || status.GatewayStarting || status.LastError != "" || status.GatewayAddress == "" {
		t.Fatalf("retry left incorrect status: %#v", status)
	}
}

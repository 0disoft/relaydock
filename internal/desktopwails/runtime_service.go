package desktopwails

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/0disoft/relaydock/internal/buildinfo"
	gatewaycomposition "github.com/0disoft/relaydock/internal/composition/gateway"
	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/localipc"
	"github.com/0disoft/relaydock/internal/transport/httpgateway"
)

type RuntimeStatus struct {
	Version         string   `json:"version"`
	IPCReady        bool     `json:"ipcReady"`
	MCPConfigured   bool     `json:"mcpConfigured"`
	GatewayStarting bool     `json:"gatewayStarting"`
	GatewayReady    bool     `json:"gatewayReady"`
	GatewayAddress  string   `json:"gatewayAddress,omitempty"`
	GatewayModels   []string `json:"gatewayModels,omitempty"`
	LastError       string   `json:"lastError,omitempty"`
}

type ProviderSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	Mode       string `json:"mode"`
}

type RuntimeService struct {
	mu sync.RWMutex

	ipcHandler localipc.Handler
	ipcServer  *localipc.Server
	ipcCancel  context.CancelFunc

	gateway         *http.Server
	gatewayListen   net.Listener
	gatewayRuntime  *gatewaycomposition.Runtime
	gatewayStarting bool
	gatewayAddress  string
	lastError       string
	mcpConfigured   bool
}

func NewRuntimeService(handler localipc.Handler) *RuntimeService {
	return &RuntimeService{ipcHandler: handler}
}

func (s *RuntimeService) Status() RuntimeStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var models []string
	if s.gatewayRuntime != nil && s.gatewayRuntime.Candidates != nil {
		models = s.gatewayRuntime.Candidates.Models()
	}
	return RuntimeStatus{
		Version:         buildinfo.Version,
		IPCReady:        s.ipcServer != nil && s.ipcServer.Ready(),
		MCPConfigured:   s.mcpConfigured,
		GatewayStarting: s.gatewayStarting,
		GatewayReady:    s.gatewayListen != nil,
		GatewayAddress:  s.gatewayAddress,
		GatewayModels:   models,
		LastError:       s.lastError,
	}
}

func (s *RuntimeService) startIPC(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.ipcServer != nil {
		s.mu.Unlock()
		return nil
	}
	if s.ipcHandler == nil {
		s.mu.Unlock()
		return fmt.Errorf("%w: local IPC handler", core.ErrInvalidConfiguration)
	}
	serverCtx, cancel := context.WithCancel(context.Background())
	server := localipc.NewServer(localipc.DefaultEndpoint(), s.ipcHandler)
	s.ipcServer = server
	s.ipcCancel = cancel
	s.lastError = ""
	s.mu.Unlock()
	go func() {
		if err := server.Run(serverCtx); err != nil {
			s.recordIPCStop(server, err)
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !server.Ready() && time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			cancel()
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !server.Ready() {
		cancel()
		return fmt.Errorf("IPC endpoint did not become ready")
	}
	return nil
}

func (s *RuntimeService) stopIPC() error {
	s.mu.Lock()
	server := s.ipcServer
	cancel := s.ipcCancel
	s.ipcServer = nil
	s.ipcCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if server != nil {
		return server.Close()
	}
	return nil
}

func (s *RuntimeService) StartLocalGateway(port int) error {
	if port == 0 {
		port = 10100
	}
	if port < 1024 || port > 65535 {
		return fmt.Errorf("%w: gateway port", core.ErrInvalidArgument)
	}
	s.mu.Lock()
	if s.gatewayListen != nil {
		s.mu.Unlock()
		return nil
	}
	if s.gatewayStarting {
		s.mu.Unlock()
		return core.ErrConflict
	}
	s.gatewayStarting = true
	s.mu.Unlock()

	providerRuntime, err := gatewaycomposition.BuildFromEnvironment()
	if err != nil {
		s.finishGatewayStart(nil, nil, nil, "", err)
		return fmt.Errorf("construct local provider runtime: %w", err)
	}
	address := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		s.finishGatewayStart(nil, nil, nil, "", err)
		return err
	}
	handler := httpgateway.NewHandlerWithOptions(httpgateway.HandlerOptions{
		Processor:   providerRuntime.Processor,
		Models:      providerRuntime.Models,
		ModelSource: providerRuntime.Candidates,
		Ready:       providerRuntime.Candidates.Ready,
		Mode:        "desktop-provider-runtime",
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	if !s.finishGatewayStart(server, listener, providerRuntime, "http://"+listener.Addr().String(), nil) {
		_ = listener.Close()
		return core.ErrConflict
	}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.recordGatewayStop(server, err)
		}
	}()
	return nil
}

func (s *RuntimeService) finishGatewayStart(server *http.Server, listener net.Listener, providerRuntime *gatewaycomposition.Runtime, address string, startErr error) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.gatewayStarting {
		return false
	}
	s.gatewayStarting = false
	if startErr != nil {
		s.lastError = startErr.Error()
		return true
	}
	if s.gatewayListen != nil {
		return false
	}
	s.gateway = server
	s.gatewayListen = listener
	s.gatewayRuntime = providerRuntime
	s.gatewayAddress = address
	s.lastError = ""
	return true
}

func (s *RuntimeService) StopLocalGateway() error {
	ctx := context.Background()
	s.mu.Lock()
	server := s.gateway
	s.gatewayStarting = false
	s.gateway = nil
	s.gatewayListen = nil
	s.gatewayRuntime = nil
	s.gatewayAddress = ""
	s.mu.Unlock()
	if server == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func (s *RuntimeService) ListProviders() []ProviderSummary {
	return []ProviderSummary{
		{ID: "local-echo", Name: "Local Echo", Configured: true, Mode: "development"},
		{ID: "openai", Name: "OpenAI", Configured: os.Getenv("OPENAI_API_KEY") != "", Mode: "official-api"},
		{ID: "anthropic", Name: "Anthropic", Configured: os.Getenv("ANTHROPIC_API_KEY") != "", Mode: "official-api"},
		{ID: "google", Name: "Google Gemini", Configured: os.Getenv("GOOGLE_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != "", Mode: "official-api"},
		{ID: "deepseek", Name: "DeepSeek", Configured: os.Getenv("DEEPSEEK_API_KEY") != "", Mode: "official-api"},
		{ID: "openrouter", Name: "OpenRouter", Configured: os.Getenv("OPENROUTER_API_KEY") != "", Mode: "official-api"},
		{ID: "openai-compatible", Name: "OpenAI Compatible", Configured: os.Getenv("OPENAI_COMPATIBLE_BASE_URL") != "", Mode: "custom-endpoint"},
	}
}

func (s *RuntimeService) MCPConfigSnippet(bridgePath string) (string, error) {
	bridgePath = strings.TrimSpace(bridgePath)
	if bridgePath == "" {
		executable, err := os.Executable()
		if err != nil {
			return "", err
		}
		name := "mcp-bridge"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		bridgePath = filepath.Join(filepath.Dir(executable), name)
	}
	absolute, err := filepath.Abs(bridgePath)
	if err != nil {
		return "", err
	}
	quoted := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`).Replace(absolute)
	s.mu.Lock()
	s.mcpConfigured = true
	s.mu.Unlock()
	return fmt.Sprintf("[mcp_servers.ai-runtime-expert]\ncommand = \"%s\"\n", quoted), nil
}

func (s *RuntimeService) recordIPCStop(server *localipc.Server, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ipcServer == server {
		s.ipcServer = nil
		s.ipcCancel = nil
	}
	if err != nil {
		s.lastError = err.Error()
	}
}

func (s *RuntimeService) recordGatewayStop(server *http.Server, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gateway == server {
		s.gateway = nil
		s.gatewayListen = nil
		s.gatewayRuntime = nil
		s.gatewayAddress = ""
	}
	if err != nil {
		s.lastError = err.Error()
	}
}

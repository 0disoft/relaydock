package desktopwails

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/credentials"
)

func TestGatewayLocalSmoke(t *testing.T) {
	clearProviderCredentialEnvironment(t)
	t.Setenv("GATEWAY_ENABLE_LOCAL_ECHO", "true")
	runGatewaySmoke(t, "local/echo")
}

func TestGatewayRoutedModelSmoke(t *testing.T) {
	clearProviderCredentialEnvironment(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || request.Model != "upstream-model" {
			t.Error("upstream received the public route name instead of the selected upstream model")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, chunk := range []string{
				`{"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
				`{"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"Checking."},"finish_reason":null}]}`,
				`{"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"RelayDock OK"},"finish_reason":null}]}`,
				`{"object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
				`[DONE]`,
			} {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"smoke","object":"chat.completion","model":"upstream-model","choices":[{"index":0,"message":{"role":"assistant","content":"RelayDock OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`)
	}))
	t.Cleanup(upstream.Close)
	t.Setenv("OPENAI_COMPATIBLE_BASE_URL", upstream.URL)
	t.Setenv("GATEWAY_ENABLE_LOCAL_ECHO", "false")
	runGatewaySmoke(t, "openai-compatible/upstream-model")
}

// Live calls are opt-in through the dedicated secret-allowlisted command.
// Never log provider bodies, transport errors, or credential values here.
func TestGatewayEntrimSmoke(t *testing.T) {
	if os.Getenv("RELAYDOCK_RUN_ENTRIM_SMOKE") != "1" {
		t.Skip("live smoke is disabled; use the dedicated opt-in command")
	}
	key := os.Getenv("ENTRIM_API_KEY")
	if key == "" {
		t.Skip("ENTRIM_API_KEY is not available; no live request sent")
	}
	clearProviderCredentialEnvironment(t)
	t.Setenv("OPENAI_COMPATIBLE_API_KEY", key)
	// The adapter appends /v1/chat/completions itself: do not duplicate /v1.
	t.Setenv("OPENAI_COMPATIBLE_BASE_URL", "https://api.entrim.ai")
	t.Setenv("GATEWAY_ENABLE_LOCAL_ECHO", "false")
	runGatewaySmoke(t, "openai-compatible/Qwen/Qwen3.6-35B-A3B")
}

func runGatewaySmoke(t *testing.T, model string) {
	t.Helper()
	for _, name := range []string{"GATEWAY_ROUTES_FILE", "GATEWAY_ROUTES_JSON", "GATEWAY_DEFAULT_PROVIDER"} {
		t.Setenv(name, "")
	}
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("cannot reserve a loopback port")
	}
	port := reserved.Addr().(*net.TCPAddr).Port
	_ = reserved.Close()
	service := NewRuntimeServiceWithCredentialStore(nil, credentials.NewMemoryStore(), nil)
	t.Cleanup(func() { _ = service.StopLocalGateway() })
	if err := service.StartLocalGateway(port); err != nil {
		t.Fatal("gateway startup failed (details suppressed)")
	}
	status := service.Status()
	if !status.GatewayReady || status.GatewayAddress == "" {
		t.Fatal("gateway did not become ready")
	}
	// No HTTP requests are running yet. Smoke never retries a paid attempt.
	service.gatewayRuntime.Gateway.MaximumAttempts = 1
	client := &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{Proxy: nil}}
	t.Cleanup(client.CloseIdleConnections)
	for _, streaming := range []bool{false, true} {
		path := "/v1/responses"
		payload := map[string]any{"model": model, "input": "Reply with exactly: RelayDock OK", "max_output_tokens": 1024}
		if streaming {
			path = "/v1/chat/completions"
			payload = map[string]any{"model": model, "messages": []any{map[string]any{"role": "user", "content": "Reply with exactly: RelayDock OK"}}, "max_tokens": 1024, "stream": true}
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal("cannot encode smoke request")
		}
		response, err := client.Post(status.GatewayAddress+path, "application/json", strings.NewReader(string(raw)))
		if err != nil {
			t.Fatal("gateway request failed (details suppressed)")
		}
		if response.StatusCode != http.StatusOK {
			var failure struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&failure)
			for _, category := range []string{"authentication", "permission", "not_found", "invalid_request", "quota_exhausted", "rate_limit", "transport", "timeout", "upstream", "unexpected EOF", "invalid character"} {
				if strings.Contains(failure.Error.Message, category) {
					t.Logf("failure category: %s", category)
				}
			}
			_ = response.Body.Close()
			t.Fatalf("gateway returned HTTP %d for %s (body suppressed)", response.StatusCode, path)
		}
		if streaming {
			checkGatewaySmokeStream(t, response, model == "openai-compatible/upstream-model")
		} else {
			var result struct {
				Status string `json:"status"`
				Output []struct {
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"output"`
			}
			err := json.NewDecoder(io.LimitReader(response.Body, 128<<10)).Decode(&result)
			_ = response.Body.Close()
			text := ""
			for _, item := range result.Output {
				for _, content := range item.Content {
					text += content.Text
				}
			}
			if err != nil || result.Status != "completed" || strings.TrimSpace(text) == "" {
				t.Fatal("Responses did not contain completed assistant text (body suppressed)")
			}
		}
		t.Logf("%s: HTTP 200 and semantic completion verified", path)
	}
	if err := service.StopLocalGateway(); err != nil {
		t.Fatal("gateway stop failed")
	}
	if service.Status().GatewayReady || service.Status().GatewayAddress != "" {
		t.Fatal("gateway status remained active after stop")
	}
	connection, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err == nil {
		_ = connection.Close()
		t.Fatal("gateway port still accepts connections after stop")
	}
}

func checkGatewaySmokeStream(t *testing.T, response *http.Response, requireReasoning bool) {
	t.Helper()
	defer response.Body.Close()
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatal("streaming response is not SSE")
	}
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 128<<10))
	text, done, finished := "", false, false
	reasoning := ""
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			done = true
			continue
		}
		var chunk struct {
			Error   json.RawMessage `json:"error"`
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					Reasoning string `json:"reasoning_content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Error) != 0 {
			t.Fatal("invalid or failed SSE event (body suppressed)")
		}
		for _, choice := range chunk.Choices {
			text += choice.Delta.Content
			reasoning += choice.Delta.Reasoning
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finished = true
			}
		}
	}
	if scanner.Err() != nil || !done || !finished || strings.TrimSpace(text) == "" {
		t.Fatal("stream did not contain assistant text, finish reason, and DONE (body suppressed)")
	}
	if requireReasoning && (reasoning != "Checking." || text != "RelayDock OK") {
		t.Fatal("same-protocol reasoning was lost or mixed into answer text")
	}
}

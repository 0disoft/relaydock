package integration_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/control/snapshot"
	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/transport/controlhttp"
	"github.com/your-org/ai-runtime-gateway/internal/transport/experthttp"
	"github.com/your-org/ai-runtime-gateway/internal/transport/httpgateway"
)

func TestGatewayServesOpenAIAnthropicAndGeminiShapes(t *testing.T) {
	server := httptest.NewServer(httpgateway.NewHandler())
	defer server.Close()

	tests := []struct {
		name, path, body, content string
	}{
		{"responses", "/v1/responses", `{"model":"local/echo","input":"hello"}`, `"object":"response"`},
		{"chat", "/v1/chat/completions", `{"model":"local/echo","messages":[{"role":"user","content":"hello"}]}`, `"object":"chat.completion"`},
		{"anthropic", "/v1/messages", `{"model":"local/echo","max_tokens":128,"messages":[{"role":"user","content":"hello"}]}`, `"type":"message"`},
		{"gemini", "/v1beta/models/local-echo:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`, `"candidates"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := postJSON(t, server.URL+tc.path, tc.body)
			defer response.Body.Close()
			payload, _ := io.ReadAll(response.Body)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.StatusCode, payload)
			}
			if !bytes.Contains(payload, []byte(tc.content)) {
				t.Fatalf("response does not contain %s: %s", tc.content, payload)
			}
			if response.Header.Get("X-AI-Runtime-Request-ID") == "" {
				t.Fatal("gateway omitted request identifier")
			}
		})
	}
}

func TestGatewayStreamsExplicitCompletion(t *testing.T) {
	server := httptest.NewServer(httpgateway.NewHandler())
	defer server.Close()
	response := postJSON(t, server.URL+"/v1/responses", `{"model":"local/echo","stream":true,"input":"hello"}`)
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.StatusCode, payload)
	}
	text := string(payload)
	events := []string{
		"response.created",
		"response.in_progress",
		"response.output_item.added",
		"response.content_part.added",
		"response.output_text.delta",
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done",
		"response.completed",
	}
	previous := -1
	for _, event := range events {
		index := strings.Index(text, "event: "+event)
		if index < 0 {
			t.Fatalf("stream omitted %s: %s", event, text)
		}
		if index <= previous {
			t.Fatalf("stream emitted %s out of order: %s", event, text)
		}
		previous = index
	}
}

func TestGatewayAnthropicStreamStartsWithoutCompletedContent(t *testing.T) {
	server := httptest.NewServer(httpgateway.NewHandler())
	defer server.Close()
	response := postJSON(t, server.URL+"/v1/messages", `{"model":"local/echo","stream":true,"max_tokens":128,"messages":[{"role":"user","content":"hello"}]}`)
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.StatusCode, payload)
	}
	text := string(payload)
	if !strings.Contains(text, `event: message_start`) || !strings.Contains(text, `"content":[]`) {
		t.Fatalf("message_start must carry an empty content array: %s", text)
	}
	if !strings.Contains(text, `event: content_block_delta`) || !strings.Contains(text, `event: message_stop`) {
		t.Fatalf("Anthropic stream omitted semantic or terminal event: %s", text)
	}
}

func TestExpertConsultationLifecycleOverHTTP(t *testing.T) {
	repository := t.TempDir()
	if err := os.WriteFile(filepath.Join(repository, "ledger.go"), []byte("package ledger\n\nfunc Capture() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(experthttp.NewHandler())
	defer server.Close()
	create := map[string]any{
		"repositoryRoot":  repository,
		"objective":       "review ledger capture idempotency",
		"candidatePaths":  []string{"ledger.go"},
		"route":           consultation.RouteOpenAIAPIPro,
		"autoApprove":     true,
		"delegationDepth": 0,
	}
	createdResponse := postValue(t, server.URL+"/v1/consultations", create)
	defer createdResponse.Body.Close()
	if createdResponse.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(createdResponse.Body)
		t.Fatalf("create status=%d body=%s", createdResponse.StatusCode, payload)
	}
	var created experthttp.CreateResponse
	if err := json.NewDecoder(createdResponse.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Consultation.State != consultation.StateQueued || created.ContextPack.ID == "" {
		t.Fatalf("consultation did not reach queued state: %+v", created)
	}

	result := map[string]any{
		"modelAttestation": "provider_response",
		"result": map[string]any{
			"decision":                "Use an append-only ledger with idempotency keys.",
			"confidence":              0.9,
			"assumptions":             []string{},
			"criticalFindings":        []any{},
			"recommendedArchitecture": map[string]any{"capture": "transactional"},
			"rejectedAlternatives":    []any{},
			"failureScenarios":        []string{"duplicate provider delivery"},
			"migrationOrder":          []string{"add unique usage key"},
			"verificationPlan":        []string{"replay duplicate usage event"},
			"unresolvedQuestions":     []string{},
			"evidenceReferences":      []string{"ledger.go"},
		},
	}
	resultResponse := postValue(t, server.URL+"/v1/consultations/"+created.Consultation.ID+"/result", result)
	defer resultResponse.Body.Close()
	if resultResponse.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resultResponse.Body)
		t.Fatalf("submit status=%d body=%s", resultResponse.StatusCode, payload)
	}

	getResponse, err := http.Get(server.URL + "/v1/consultations/" + created.Consultation.ID)
	if err != nil {
		t.Fatalf("get consultation: %v", err)
	}
	defer getResponse.Body.Close()
	var final experthttp.ConsultationResponse
	if err := json.NewDecoder(getResponse.Body).Decode(&final); err != nil {
		t.Fatalf("decode final consultation: %v", err)
	}
	if final.Consultation.State != consultation.StateCompleted || final.Result == nil {
		t.Fatalf("consultation did not complete with result: %+v", final)
	}
}

func TestControlPlanePublishesVerifiableSnapshots(t *testing.T) {
	api, err := controlhttp.NewAPI()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	defer server.Close()

	keyResponse, err := http.Get(server.URL + "/v1/signing-key")
	if err != nil {
		t.Fatal(err)
	}
	defer keyResponse.Body.Close()
	var keyBody map[string]string
	if err := json.NewDecoder(keyResponse.Body).Decode(&keyBody); err != nil {
		t.Fatal(err)
	}
	public, err := base64.RawURLEncoding.DecodeString(keyBody["publicKey"])
	if err != nil || len(public) != ed25519.PublicKeySize {
		t.Fatalf("invalid signing key: %v length=%d", err, len(public))
	}

	publish := map[string]any{
		"expiresInSeconds": 3600,
		"models": []map[string]any{{
			"virtualModel": "code-deep",
			"candidates":   []string{"openai/gpt-test"},
			"policyId":     "strict",
		}},
	}
	response := postValue(t, server.URL+"/v1/snapshot", publish)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("publish status=%d body=%s", response.StatusCode, payload)
	}
	var published snapshot.Snapshot
	if err := json.NewDecoder(response.Body).Decode(&published); err != nil {
		t.Fatal(err)
	}
	if published.Revision != 2 || published.ExpiresAt.Before(time.Now()) {
		t.Fatalf("unexpected published snapshot: %+v", published)
	}
	verifier := snapshot.NewEd25519Signer(nil, ed25519.PublicKey(public))
	if err := verifier.Verify(context.Background(), published); err != nil {
		t.Fatalf("snapshot signature invalid: %v", err)
	}
}

func postJSON(t *testing.T, endpoint, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func postValue(t *testing.T, endpoint string, value any) *http.Response {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return postJSON(t, endpoint, string(payload))
}

package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/your-org/ai-runtime-gateway/internal/transport/apiutil"
	"github.com/your-org/ai-runtime-gateway/internal/transport/httpgateway"
)

func TestGatewayRejectsOversizedRequestWith413(t *testing.T) {
	server := httptest.NewServer(httpgateway.NewHandlerWithOptions(httpgateway.HandlerOptions{MaximumBodyBytes: 64}))
	defer server.Close()

	body := `{"model":"local/echo","input":"` + strings.Repeat("x", 128) + `"}`
	response := postJSON(t, server.URL+"/v1/responses", body)
	defer response.Body.Close()
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("status=%d body=%s", response.StatusCode, payload)
	}
	var failure apiutil.ErrorBody
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if failure.Error.Code != "payload_too_large" {
		t.Fatalf("unexpected error code: %+v", failure)
	}
}

func TestGatewayRejectsMalformedTrailingJSON(t *testing.T) {
	server := httptest.NewServer(httpgateway.NewHandler())
	defer server.Close()

	for _, body := range []string{
		`{"model":"local/echo","input":"hello"} trailing`,
		`{"model":"local/echo","input":"hello"} {"second":true}`,
	} {
		response := postJSON(t, server.URL+"/v1/responses", body)
		payload, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("body=%q status=%d response=%s", body, response.StatusCode, payload)
		}
		if !strings.Contains(string(payload), "invalid_request") {
			t.Fatalf("body=%q missing invalid_request: %s", body, payload)
		}
	}
}

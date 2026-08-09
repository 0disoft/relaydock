package httpgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRuntimeStatusEndpointReportsModeAndReadiness(t *testing.T) {
	handler := NewHandlerWithOptions(HandlerOptions{
		Ready: func() bool { return false },
		Mode:  "test-runtime",
		RuntimeStatus: func(context.Context) (map[string]any, error) {
			return map[string]any{"revision": 7}, nil
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/status", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["mode"] != "test-runtime" || body["ready"] != false || body["revision"] != float64(7) {
		t.Fatalf("unexpected runtime status: %#v", body)
	}
}

func TestRuntimeStatusEndpointIsAbsentWithoutSource(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/status", nil)
	response := httptest.NewRecorder()
	NewHandler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

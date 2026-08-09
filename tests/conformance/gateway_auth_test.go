package conformance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/auth/authorization"
	"github.com/0disoft/relaydock/internal/auth/virtualkey"
	"github.com/0disoft/relaydock/internal/transport/apiutil"
	"github.com/0disoft/relaydock/internal/transport/httpgateway"
)

func TestGatewayVirtualKeyFiltersModelsAndRejectsDisallowedInvocation(t *testing.T) {
	authenticator := virtualkey.NewMemoryAuthenticator([]byte("01234567890123456789012345678901"))
	raw, _, err := authenticator.Issue(
		"live", "tenant", "project",
		[]string{authorization.ScopeModelsInvoke}, []string{"allowed/model"}, zeroTime(),
	)
	if err != nil {
		t.Fatal(err)
	}
	handler := httpgateway.NewHandlerWithOptions(httpgateway.HandlerOptions{
		Processor: httpgateway.EchoProcessor{},
		Models:    []string{"allowed/model", "blocked/model"},
	})
	handler = apiutil.RequireGatewayAuthentication("", authenticator, handler, "/healthz", "/readyz")

	modelsRequest := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	modelsRequest.Header.Set("Authorization", "Bearer "+raw)
	modelsResponse := httptest.NewRecorder()
	handler.ServeHTTP(modelsResponse, modelsRequest)
	if modelsResponse.Code != http.StatusOK {
		t.Fatalf("models status=%d body=%s", modelsResponse.Code, modelsResponse.Body.String())
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(modelsResponse.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Data[0].ID != "allowed/model" {
		t.Fatalf("unexpected filtered models: %+v", list.Data)
	}

	blockedRequest := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"blocked/model","input":"test"}`))
	blockedRequest.Header.Set("Authorization", "Bearer "+raw)
	blockedRequest.Header.Set("Content-Type", "application/json")
	blockedResponse := httptest.NewRecorder()
	handler.ServeHTTP(blockedResponse, blockedRequest)
	if blockedResponse.Code != http.StatusForbidden {
		t.Fatalf("blocked status=%d body=%s", blockedResponse.Code, blockedResponse.Body.String())
	}

	allowedRequest := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"allowed/model","input":"test"}`))
	allowedRequest.Header.Set("Authorization", "Bearer "+raw)
	allowedRequest.Header.Set("Content-Type", "application/json")
	allowedResponse := httptest.NewRecorder()
	handler.ServeHTTP(allowedResponse, allowedRequest)
	if allowedResponse.Code != http.StatusOK {
		t.Fatalf("allowed status=%d body=%s", allowedResponse.Code, allowedResponse.Body.String())
	}
}

func TestGatewayAuthenticationRejectsKeyWithoutInvokeScope(t *testing.T) {
	authenticator := virtualkey.NewMemoryAuthenticator([]byte("01234567890123456789012345678901"))
	raw, _, err := authenticator.Issue("live", "tenant", "project", []string{"consultations:read"}, nil, zeroTime())
	if err != nil {
		t.Fatal(err)
	}
	handler := apiutil.RequireGatewayAuthentication("", authenticator, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unauthorized request reached handler")
	}))
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	request.Header.Set("Authorization", "Bearer "+raw)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestGatewayOperatorBearerInjectsInvokePrincipal(t *testing.T) {
	handler := apiutil.RequireGatewayAuthentication("operator-token", nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := virtualkey.PrincipalFromContext(r.Context())
		if !ok || principal.Authorize(authorization.ScopeModelsInvoke, "any/model") != nil {
			t.Fatal("operator principal missing invoke scope")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/private", nil)
	request.Header.Set("Authorization", "Bearer operator-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d", response.Code)
	}
}

func zeroTime() (value time.Time) { return value }

var _ virtualkey.Authenticator = virtualkey.NewMemoryAuthenticator(nil)

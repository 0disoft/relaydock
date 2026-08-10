package serversecrets

import (
	"context"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestGCPSecretManagerUsesWorkloadIdentityAndVerifiesPayload(t *testing.T) {
	secret := []byte("provider-key")
	crc := crc32.Checksum(secret, crc32.MakeTable(crc32.Castagnoli))
	requests := 0
	provider := newGCPSecretManager(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		switch request.URL.String() {
		case "http://metadata.test/token":
			if request.Header.Get("Metadata-Flavor") != "Google" || request.Header.Get("Authorization") != "" {
				t.Fatalf("unexpected metadata headers: %#v", request.Header)
			}
			return response(http.StatusOK, `{"access_token":"workload-token","token_type":"Bearer","expires_in":3599}`, map[string]string{"Metadata-Flavor": "Google"}), nil
		case "https://secret.test/v1/projects/project-1/secrets/openai-key/versions/latest:access":
			if request.Header.Get("Authorization") != "Bearer workload-token" {
				t.Fatalf("authorization=%q", request.Header.Get("Authorization"))
			}
			body := fmt.Sprintf(`{"payload":{"data":%q,"dataCrc32c":%q}}`, base64.StdEncoding.EncodeToString(secret), fmt.Sprint(crc))
			return response(http.StatusOK, body, nil), nil
		default:
			t.Fatalf("unexpected request URL %s", request.URL)
			return nil, nil
		}
	}), "http://metadata.test/token", "https://secret.test/v1/")

	value, err := provider.Resolve(context.Background(), "projects/project-1/secrets/openai-key/versions/latest")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || string(value) != string(secret) {
		t.Fatalf("requests=%d value=%q", requests, value)
	}
}

func TestGCPSecretManagerRejectsCorruptPayloadWithoutExposingBodies(t *testing.T) {
	provider := newGCPSecretManager(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "metadata.test" {
			return response(http.StatusOK, `{"access_token":"sensitive-workload-token","token_type":"Bearer","expires_in":3599}`, map[string]string{"Metadata-Flavor": "Google"}), nil
		}
		return response(http.StatusOK, `{"payload":{"data":"c2Vuc2l0aXZlLXByb3ZpZGVyLWtleQ==","dataCrc32c":"1"}}`, nil), nil
	}), "http://metadata.test/token", "https://secret.test/v1/")

	_, err := provider.Resolve(context.Background(), "projects/project-1/secrets/openai-key/versions/latest")
	if err == nil {
		t.Fatal("expected checksum failure")
	}
	for _, sensitive := range []string{"sensitive-workload-token", "sensitive-provider-key"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("error exposed secret material: %v", err)
		}
	}
}

func TestGCPSecretManagerRejectsRedirectsAndMalformedResources(t *testing.T) {
	provider := newGCPSecretManager(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusFound, "sensitive redirect body", map[string]string{"Location": "https://attacker.example"}), nil
	}), "http://metadata.test/token", "https://secret.test/v1/")

	if _, err := provider.Resolve(context.Background(), "projects/project-1/secrets/openai-key/versions/latest"); err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("redirect error=%v", err)
	}
	for _, resource := range []string{"", "projects/p/secrets/s", "projects/../secrets/s/versions/latest", "projects/p/secrets/a/b/versions/latest"} {
		if _, err := provider.Resolve(context.Background(), resource); err == nil {
			t.Fatalf("accepted malformed resource %q", resource)
		}
	}
}

func TestGCPSecretManagerMetadataTransportDoesNotUseEnvironmentProxy(t *testing.T) {
	provider := NewGCPSecretManager()
	transport, ok := provider.metadataClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("metadata transport=%T", provider.metadataClient.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("metadata transport must not use an environment proxy")
	}
}

func response(status int, body string, headers map[string]string) *http.Response {
	response := &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	for name, value := range headers {
		response.Header.Set(name, value)
	}
	return response
}

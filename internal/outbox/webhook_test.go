package outbox

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

func TestWebhookPublisherSignsStableEnvelope(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	now := time.Unix(1_700_000_000, 0).UTC()
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		capturedBody, _ = io.ReadAll(request.Body)
		if request.Header.Get("Idempotency-Key") != "evt-1" || request.Header.Get("X-AI-Runtime-Event-Topic") != "runtime.request.finished" {
			t.Errorf("missing delivery headers: %v", request.Header)
		}
		timestamp := request.Header.Get("X-AI-Runtime-Timestamp")
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write([]byte(timestamp + "."))
		_, _ = mac.Write(capturedBody)
		expected := "v1=" + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(expected), []byte(request.Header.Get("X-AI-Runtime-Signature"))) {
			t.Errorf("invalid signature: %q expected %q", request.Header.Get("X-AI-Runtime-Signature"), expected)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	publisher, err := NewWebhookPublisher(WebhookConfig{Endpoint: server.URL, Secret: secret, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	event := Event{ID: "evt-1", Topic: "runtime.request.finished", AggregateID: "req-1", Payload: []byte(`{"state":"completed"}`), AvailableAt: now, Attempts: 2}
	if err := publisher.Publish(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	var envelope webhookEnvelope
	if err := json.Unmarshal(capturedBody, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.DeliveryAttempt != 3 || envelope.ID != event.ID || envelope.AggregateID != event.AggregateID {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
}

func TestWebhookPublisherPreservesRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "17")
		http.Error(w, "busy", http.StatusTooManyRequests)
	}))
	defer server.Close()
	publisher := newWebhookTestPublisher(t, server.URL)
	err := publisher.Publish(context.Background(), Event{ID: "evt", Topic: "topic", AggregateID: "aggregate", Payload: []byte(`{}`)})
	var retry RetryAfterError
	if !errors.As(err, &retry) || retry.After != 17*time.Second {
		t.Fatalf("unexpected retry error: %#v", err)
	}
}

func TestWebhookPublisherTreatsValidationFailureAsPermanent(t *testing.T) {
	publisher := newWebhookTestPublisher(t, "http://127.0.0.1:1")
	err := publisher.Publish(context.Background(), Event{ID: "evt", Topic: "topic", AggregateID: "aggregate", Payload: []byte(`not-json`)})
	if !IsPermanentPublishError(err) {
		t.Fatalf("expected permanent error, got %v", err)
	}
}

func TestWebhookPublisherClassifiesClientFailures(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		permanent bool
	}{
		{name: "bad request", status: http.StatusBadRequest, permanent: true},
		{name: "unacknowledged conflict", status: http.StatusConflict, permanent: true},
		{name: "server error", status: http.StatusServiceUnavailable, permanent: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "response", test.status)
			}))
			defer server.Close()
			publisher := newWebhookTestPublisher(t, server.URL)
			err := publisher.Publish(context.Background(), Event{ID: "evt", Topic: "topic", AggregateID: "aggregate", Payload: []byte(`{}`)})
			if err == nil {
				t.Fatal("expected failure")
			}
			if IsPermanentPublishError(err) != test.permanent {
				t.Fatalf("permanent=%v error=%v", IsPermanentPublishError(err), err)
			}
		})
	}
}

func TestWebhookPublisherAcceptsExplicitDuplicateAcknowledgement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set(WebhookDeliveryStatusHeader, WebhookDeliveryDuplicate)
		w.Header().Set(WebhookEventIDHeader, request.Header.Get(WebhookEventIDHeader))
		http.Error(w, "duplicate", http.StatusConflict)
	}))
	defer server.Close()
	publisher := newWebhookTestPublisher(t, server.URL)
	if err := publisher.Publish(context.Background(), Event{ID: "evt", Topic: "topic", AggregateID: "aggregate", Payload: []byte(`{}`)}); err != nil {
		t.Fatalf("explicit duplicate acknowledgement failed: %v", err)
	}
}

func TestWebhookPublisherRejectsDuplicateAcknowledgementForAnotherEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(WebhookDeliveryStatusHeader, WebhookDeliveryDuplicate)
		w.Header().Set(WebhookEventIDHeader, "different-event")
		http.Error(w, "duplicate", http.StatusConflict)
	}))
	defer server.Close()
	publisher := newWebhookTestPublisher(t, server.URL)
	err := publisher.Publish(context.Background(), Event{ID: "evt", Topic: "topic", AggregateID: "aggregate", Payload: []byte(`{}`)})
	if !IsPermanentPublishError(err) {
		t.Fatalf("mismatched duplicate acknowledgement must be permanent: %v", err)
	}
}

func TestWebhookPublisherRejectsInsecureRemoteEndpoint(t *testing.T) {
	_, err := NewWebhookPublisher(WebhookConfig{
		Endpoint: "http://example.com/hooks/runtime",
		Secret:   []byte("0123456789abcdef0123456789abcdef"),
	})
	if !errors.Is(err, core.ErrInvalidConfiguration) || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("unexpected configuration error: %v", err)
	}
}

func newWebhookTestPublisher(t *testing.T, endpoint string) *WebhookPublisher {
	t.Helper()
	publisher, err := NewWebhookPublisher(WebhookConfig{
		Endpoint: endpoint,
		Secret:   []byte("0123456789abcdef0123456789abcdef"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return publisher
}

func TestVerifyWebhookSignature(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	now := time.Unix(1_700_000_000, 0).UTC()
	body := []byte(`{"id":"evt-1"}`)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	signature := "v1=" + signWebhookPayload(secret, timestamp, body)

	if err := VerifyWebhookSignature(secret, timestamp, signature, body, WebhookVerificationOptions{Now: now}); err != nil {
		t.Fatalf("verify valid signature: %v", err)
	}
	if err := VerifyWebhookSignature(secret, timestamp, signature, []byte(`{"id":"evt-2"}`), WebhookVerificationOptions{Now: now}); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("expected tampered body rejection, got %v", err)
	}
}

func TestVerifyWebhookSignatureRejectsReplayAndFutureTimestamp(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	now := time.Unix(1_700_000_000, 0).UTC()
	body := []byte(`{"id":"evt-1"}`)

	staleTimestamp := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)
	staleSignature := "v1=" + signWebhookPayload(secret, staleTimestamp, body)
	if err := VerifyWebhookSignature(secret, staleTimestamp, staleSignature, body, WebhookVerificationOptions{Now: now, MaximumAge: 5 * time.Minute}); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("expected stale signature rejection, got %v", err)
	}

	futureTimestamp := strconv.FormatInt(now.Add(2*time.Minute).Unix(), 10)
	futureSignature := "v1=" + signWebhookPayload(secret, futureTimestamp, body)
	if err := VerifyWebhookSignature(secret, futureTimestamp, futureSignature, body, WebhookVerificationOptions{Now: now, MaximumFutureSkew: time.Minute}); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("expected future signature rejection, got %v", err)
	}
}

func TestVerifyWebhookSignatureRejectsMalformedInput(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	body := []byte(`{}`)
	for _, test := range []struct {
		timestamp string
		signature string
	}{
		{"", ""},
		{"not-a-number", "v1=deadbeef"},
		{"1700000000", "v2=deadbeef"},
		{"1700000000", "v1=not-hex"},
	} {
		if err := VerifyWebhookSignature(secret, test.timestamp, test.signature, body, WebhookVerificationOptions{Now: time.Unix(1_700_000_000, 0)}); !errors.Is(err, core.ErrUnauthorized) {
			t.Fatalf("expected malformed signature rejection for %#v, got %v", test, err)
		}
	}
}

func TestWebhookPublisherNeverFollowsRedirectsFromCustomClient(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	publisher, err := NewWebhookPublisher(WebhookConfig{
		Endpoint:   source.URL,
		Secret:     []byte("0123456789abcdef0123456789abcdef"),
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = publisher.Publish(context.Background(), Event{ID: "evt", Topic: "topic", AggregateID: "aggregate", Payload: []byte(`{}`)})
	if !IsPermanentPublishError(err) {
		t.Fatalf("redirect must be rejected as permanent: %v", err)
	}
	if redirected {
		t.Fatal("custom HTTP client followed a webhook redirect")
	}
}

package webhooksink

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/outbox"
)

func TestHandlerAcceptsAndDeduplicatesSignedEvent(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	now := time.Unix(1_700_000_000, 0).UTC()
	sink, err := New(Config{Secret: secret, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(sink.HTTPHandler())
	defer server.Close()

	publisher, err := outbox.NewWebhookPublisher(outbox.WebhookConfig{
		Endpoint:          server.URL + "/events",
		Secret:            secret,
		AllowInsecureHTTP: true,
		Now:               func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	event := outbox.Event{ID: "evt-1", Topic: "runtime.request.finished", AggregateID: "req-1", Payload: json.RawMessage(`{"ok":true}`), AvailableAt: now}
	if err := publisher.Publish(context.Background(), event); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if err := publisher.Publish(context.Background(), event); err != nil {
		t.Fatalf("duplicate delivery should be acknowledged: %v", err)
	}
	if count := sink.count(); count != 1 {
		t.Fatalf("unexpected receipt count: %d", count)
	}
}

func TestHandlerRejectsHeaderBodyMismatch(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	now := time.Unix(1_700_000_000, 0).UTC()
	sink, err := New(Config{Secret: secret, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"id":"evt-1","topic":"topic","aggregateId":"aggregate","payload":{},"availableAt":"2023-11-14T22:13:20Z","deliveryAttempt":1}`)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	signature := "v1=" + testSignature(secret, timestamp, body)
	request := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
	request.Header.Set("X-AI-Runtime-Timestamp", timestamp)
	request.Header.Set("X-AI-Runtime-Signature", signature)
	request.Header.Set("X-AI-Runtime-Event-ID", "evt-other")
	request.Header.Set("Idempotency-Key", "evt-1")
	request.Header.Set("X-AI-Runtime-Event-Topic", "topic")
	response := httptest.NewRecorder()
	sink.HTTPHandler().ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
	}
}

func testSignature(secret []byte, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestReceiptListIsDeterministicNewestFirst(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	now := time.Unix(1_700_000_000, 0).UTC()
	sink, err := New(Config{Secret: secret, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	sink.record(Receipt{ID: "evt-a", Topic: "topic", Digest: "a", ReceivedAt: now.Add(-time.Second)})
	sink.record(Receipt{ID: "evt-b", Topic: "topic", Digest: "b", ReceivedAt: now})
	sink.record(Receipt{ID: "evt-c", Topic: "topic", Digest: "c", ReceivedAt: now})

	request := httptest.NewRequest(http.MethodGet, "/v1/receipts?limit=2", nil)
	response := httptest.NewRecorder()
	sink.HTTPHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Receipts []Receipt `json:"receipts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode receipt list: %v", err)
	}
	if len(payload.Receipts) != 2 || payload.Receipts[0].ID != "evt-c" || payload.Receipts[1].ID != "evt-b" {
		t.Fatalf("unexpected deterministic order: %#v", payload.Receipts)
	}
}

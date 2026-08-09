package outbox

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

const defaultWebhookMaximumResponseBytes int64 = 32 << 10

const (
	WebhookEventIDHeader        = "X-AI-Runtime-Event-ID"
	WebhookDeliveryStatusHeader = "X-AI-Runtime-Delivery-Status"
	WebhookDeliveryDuplicate    = "duplicate"
)

type WebhookConfig struct {
	Endpoint             string
	Secret               []byte
	HTTPClient           *http.Client
	UserAgent            string
	MaximumResponseBytes int64
	AllowInsecureHTTP    bool
	Headers              map[string]string
	Now                  func() time.Time
}

type WebhookPublisher struct {
	endpoint             string
	secret               []byte
	client               *http.Client
	userAgent            string
	maximumResponseBytes int64
	headers              map[string]string
	now                  func() time.Time
}

type webhookEnvelope struct {
	ID              string          `json:"id"`
	Topic           string          `json:"topic"`
	AggregateID     string          `json:"aggregateId"`
	Payload         json.RawMessage `json:"payload"`
	AvailableAt     time.Time       `json:"availableAt"`
	DeliveryAttempt int             `json:"deliveryAttempt"`
}

func NewWebhookPublisher(config WebhookConfig) (*WebhookPublisher, error) {
	endpoint, err := validateWebhookEndpoint(config.Endpoint, config.AllowInsecureHTTP)
	if err != nil {
		return nil, err
	}
	if len(config.Secret) < 32 {
		return nil, fmt.Errorf("%w: outbox webhook secret must contain at least 32 bytes", core.ErrInvalidConfiguration)
	}
	if config.MaximumResponseBytes <= 0 {
		config.MaximumResponseBytes = defaultWebhookMaximumResponseBytes
	}
	if config.MaximumResponseBytes > 1<<20 {
		return nil, fmt.Errorf("%w: outbox webhook response limit exceeds 1 MiB", core.ErrInvalidConfiguration)
	}
	config.UserAgent = strings.TrimSpace(config.UserAgent)
	if config.UserAgent == "" {
		config.UserAgent = "ai-runtime-gateway-outbox/unknown"
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	client := config.HTTPClient
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.ResponseHeaderTimeout = 20 * time.Second
		transport.ExpectContinueTimeout = time.Second
		client = &http.Client{Transport: transport}
	} else {
		clone := *client
		client = &clone
	}
	// Never follow redirects, even when an operator supplies a custom client.
	// The signature covers the original endpoint and redirect following can
	// otherwise turn a valid webhook into an SSRF or credential-forwarding hop.
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	headers := make(map[string]string, len(config.Headers))
	for name, value := range config.Headers {
		name = http.CanonicalHeaderKey(strings.TrimSpace(name))
		if name == "" || strings.TrimSpace(value) == "" || reservedWebhookHeader(name) {
			continue
		}
		headers[name] = strings.TrimSpace(value)
	}
	return &WebhookPublisher{
		endpoint:             endpoint,
		secret:               append([]byte(nil), config.Secret...),
		client:               client,
		userAgent:            config.UserAgent,
		maximumResponseBytes: config.MaximumResponseBytes,
		headers:              headers,
		now:                  config.Now,
	}, nil
}

func (p *WebhookPublisher) Publish(ctx context.Context, event Event) error {
	if err := validatePublishEvent(event); err != nil {
		return PermanentError{Err: err}
	}
	envelope := webhookEnvelope{
		ID:              event.ID,
		Topic:           event.Topic,
		AggregateID:     event.AggregateID,
		Payload:         append(json.RawMessage(nil), event.Payload...),
		AvailableAt:     event.AvailableAt.UTC(),
		DeliveryAttempt: event.Attempts + 1,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return PermanentError{Err: fmt.Errorf("encode outbox webhook: %w", err)}
	}
	timestamp := strconv.FormatInt(p.now().Unix(), 10)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return PermanentError{Err: fmt.Errorf("create outbox webhook request: %w", err)}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", p.userAgent)
	request.Header.Set("Idempotency-Key", event.ID)
	request.Header.Set(WebhookEventIDHeader, event.ID)
	request.Header.Set("X-AI-Runtime-Event-Topic", event.Topic)
	request.Header.Set("X-AI-Runtime-Timestamp", timestamp)
	request.Header.Set("X-AI-Runtime-Signature", "v1="+signWebhookPayload(p.secret, timestamp, body))
	for name, value := range p.headers {
		request.Header.Set(name, value)
	}

	response, err := p.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("deliver outbox webhook: %w", err)
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, p.maximumResponseBytes+1))
	if readErr != nil {
		return fmt.Errorf("read outbox webhook response: %w", readErr)
	}
	if int64(len(responseBody)) > p.maximumResponseBytes {
		responseBody = responseBody[:p.maximumResponseBytes]
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	if response.StatusCode == http.StatusConflict &&
		strings.EqualFold(strings.TrimSpace(response.Header.Get(WebhookDeliveryStatusHeader)), WebhookDeliveryDuplicate) &&
		strings.TrimSpace(response.Header.Get(WebhookEventIDHeader)) == event.ID {
		return nil
	}

	statusError := fmt.Errorf("outbox webhook returned %s: %s", response.Status, boundedResponseText(responseBody))
	if retryableWebhookStatus(response.StatusCode) {
		if after := parseRetryAfter(response.Header.Get("Retry-After"), p.now()); after > 0 {
			return RetryAfterError{Err: statusError, After: after}
		}
		return statusError
	}
	return PermanentError{Err: statusError}
}

func validateWebhookEndpoint(raw string, allowInsecure bool) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: invalid outbox webhook URL", core.ErrInvalidConfiguration)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%w: outbox webhook URL must not contain user information", core.ErrInvalidConfiguration)
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("%w: outbox webhook URL must not contain a fragment", core.ErrInvalidConfiguration)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
	case "http":
		if !allowInsecure && !loopbackHostname(parsed.Hostname()) {
			return "", fmt.Errorf("%w: non-loopback outbox webhook requires HTTPS", core.ErrInvalidConfiguration)
		}
	default:
		return "", fmt.Errorf("%w: outbox webhook scheme must be HTTP or HTTPS", core.ErrInvalidConfiguration)
	}
	return parsed.String(), nil
}

func validatePublishEvent(event Event) error {
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Topic) == "" || strings.TrimSpace(event.AggregateID) == "" {
		return fmt.Errorf("%w: incomplete outbox event", core.ErrInvalidArgument)
	}
	if !json.Valid(event.Payload) {
		return fmt.Errorf("%w: outbox payload is not valid JSON", core.ErrInvalidArgument)
	}
	if event.Attempts < 0 {
		return fmt.Errorf("%w: negative outbox attempt count", core.ErrInvalidArgument)
	}
	return nil
}

type WebhookVerificationOptions struct {
	Now               time.Time
	MaximumAge        time.Duration
	MaximumFutureSkew time.Duration
}

// VerifyWebhookSignature validates the delivery signature and its timestamp.
// Consumers should perform duplicate suppression using X-AI-Runtime-Event-ID
// after this check; signature verification alone does not provide replay
// protection.
func VerifyWebhookSignature(secret []byte, timestamp, signature string, body []byte, options WebhookVerificationOptions) error {
	if len(secret) < 32 {
		return fmt.Errorf("%w: webhook verification secret must contain at least 32 bytes", core.ErrInvalidConfiguration)
	}
	timestamp = strings.TrimSpace(timestamp)
	signature = strings.TrimSpace(signature)
	if timestamp == "" || !strings.HasPrefix(signature, "v1=") {
		return core.ErrUnauthorized
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || seconds <= 0 {
		return core.ErrUnauthorized
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, "v1="))
	if err != nil || len(provided) != sha256.Size {
		return core.ErrUnauthorized
	}
	expected, err := hex.DecodeString(signWebhookPayload(secret, timestamp, body))
	if err != nil || !hmac.Equal(provided, expected) {
		return core.ErrUnauthorized
	}
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	} else {
		options.Now = options.Now.UTC()
	}
	if options.MaximumAge <= 0 {
		options.MaximumAge = 5 * time.Minute
	}
	if options.MaximumFutureSkew <= 0 {
		options.MaximumFutureSkew = time.Minute
	}
	signedAt := time.Unix(seconds, 0).UTC()
	if signedAt.After(options.Now.Add(options.MaximumFutureSkew)) || options.Now.Sub(signedAt) > options.MaximumAge {
		return core.ErrUnauthorized
	}
	return nil
}

func signWebhookPayload(secret []byte, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func retryableWebhookStatus(status int) bool {
	return status == http.StatusRequestTimeout ||
		status == http.StatusTooEarly ||
		status == http.StatusTooManyRequests ||
		status >= http.StatusInternalServerError
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	at, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	if delay := at.Sub(now); delay > 0 {
		return delay
	}
	return 0
}

func boundedResponseText(value []byte) string {
	text := strings.TrimSpace(string(value))
	if text == "" {
		return "empty response"
	}
	const maximum = 1_024
	if len(text) > maximum {
		return text[:maximum] + "…"
	}
	return text
}

func loopbackHostname(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func reservedWebhookHeader(name string) bool {
	switch strings.ToLower(name) {
	case "content-type", "accept", "user-agent", "idempotency-key",
		"x-ai-runtime-event-id", "x-ai-runtime-event-topic",
		"x-ai-runtime-timestamp", "x-ai-runtime-signature", "host",
		"content-length":
		return true
	default:
		return false
	}
}

func IsPermanentPublishError(err error) bool {
	var permanent PermanentError
	return errors.As(err, &permanent)
}

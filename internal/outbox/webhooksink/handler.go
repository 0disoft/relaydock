package webhooksink

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/outbox"
)

const defaultMaximumBodyBytes int64 = 2 << 20

type Config struct {
	Secret            []byte
	MaximumBodyBytes  int64
	MaximumAge        time.Duration
	MaximumFutureSkew time.Duration
	Retention         time.Duration
	MaximumReceipts   int
	Now               func() time.Time
	Logger            *slog.Logger
}

type Envelope struct {
	ID              string          `json:"id"`
	Topic           string          `json:"topic"`
	AggregateID     string          `json:"aggregateId"`
	Payload         json.RawMessage `json:"payload"`
	AvailableAt     time.Time       `json:"availableAt"`
	DeliveryAttempt int             `json:"deliveryAttempt"`
}

type Receipt struct {
	ID         string    `json:"id"`
	Topic      string    `json:"topic"`
	Digest     string    `json:"digest"`
	ReceivedAt time.Time `json:"receivedAt"`
}

type Handler struct {
	config   Config
	mu       sync.Mutex
	receipts map[string]Receipt
}

func New(config Config) (*Handler, error) {
	if len(config.Secret) < 32 {
		return nil, fmt.Errorf("%w: webhook sink secret must contain at least 32 bytes", core.ErrInvalidConfiguration)
	}
	if config.MaximumBodyBytes <= 0 {
		config.MaximumBodyBytes = defaultMaximumBodyBytes
	}
	if config.MaximumBodyBytes > 64<<20 {
		return nil, fmt.Errorf("%w: webhook sink body limit exceeds 64 MiB", core.ErrInvalidConfiguration)
	}
	if config.MaximumAge <= 0 {
		config.MaximumAge = 5 * time.Minute
	}
	if config.MaximumFutureSkew <= 0 {
		config.MaximumFutureSkew = time.Minute
	}
	if config.Retention <= 0 {
		config.Retention = 24 * time.Hour
	}
	if config.MaximumReceipts <= 0 {
		config.MaximumReceipts = 100_000
	}
	if config.MaximumReceipts > 1_000_000 {
		return nil, fmt.Errorf("%w: webhook sink receipt limit exceeds one million", core.ErrInvalidConfiguration)
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	config.Secret = append([]byte(nil), config.Secret...)
	return &Handler{config: config, receipts: make(map[string]Receipt)}, nil
}

func (h *Handler) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /v1/receipts", h.list)
	mux.HandleFunc("POST /events", h.receive)
	return mux
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "receipts": h.count()})
}

func (h *Handler) list(w http.ResponseWriter, request *http.Request) {
	limit := 100
	if raw := strings.TrimSpace(request.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 1_000 {
			writeError(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		limit = parsed
	}
	h.mu.Lock()
	h.pruneLocked(h.now())
	values := make([]Receipt, 0, len(h.receipts))
	for _, receipt := range h.receipts {
		values = append(values, receipt)
	}
	h.mu.Unlock()
	sort.Slice(values, func(i, j int) bool {
		if values[i].ReceivedAt.Equal(values[j].ReceivedAt) {
			return values[i].ID > values[j].ID
		}
		return values[i].ReceivedAt.After(values[j].ReceivedAt)
	})
	if len(values) > limit {
		values = values[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"receipts": values})
}

func (h *Handler) receive(w http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, request.Body, h.config.MaximumBodyBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "body_too_large")
		return
	}
	timestamp := request.Header.Get("X-AI-Runtime-Timestamp")
	signature := request.Header.Get("X-AI-Runtime-Signature")
	if err := outbox.VerifyWebhookSignature(h.config.Secret, timestamp, signature, body, outbox.WebhookVerificationOptions{
		Now:               h.now(),
		MaximumAge:        h.config.MaximumAge,
		MaximumFutureSkew: h.config.MaximumFutureSkew,
	}); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_signature")
		return
	}
	var envelope Envelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || ensureEOF(decoder) != nil {
		writeError(w, http.StatusBadRequest, "invalid_envelope")
		return
	}
	if err := validateEnvelope(request, envelope); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	digestBytes := sha256.Sum256(body)
	receipt := Receipt{
		ID:         envelope.ID,
		Topic:      envelope.Topic,
		Digest:     hex.EncodeToString(digestBytes[:]),
		ReceivedAt: h.now(),
	}
	duplicate, conflict := h.record(receipt)
	if conflict {
		writeError(w, http.StatusUnprocessableEntity, "idempotency_conflict")
		return
	}
	if duplicate {
		w.Header().Set(outbox.WebhookDeliveryStatusHeader, outbox.WebhookDeliveryDuplicate)
		w.Header().Set(outbox.WebhookEventIDHeader, receipt.ID)
		writeJSON(w, http.StatusConflict, map[string]any{"status": "duplicate", "id": receipt.ID})
		return
	}
	if h.config.Logger != nil {
		h.config.Logger.InfoContext(request.Context(), "received reference webhook", "eventId", receipt.ID, "topic", receipt.Topic, "attempt", envelope.DeliveryAttempt)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "id": receipt.ID})
}

func validateEnvelope(request *http.Request, value Envelope) error {
	value.ID = strings.TrimSpace(value.ID)
	value.Topic = strings.TrimSpace(value.Topic)
	value.AggregateID = strings.TrimSpace(value.AggregateID)
	if value.ID == "" || value.Topic == "" || value.AggregateID == "" || value.DeliveryAttempt <= 0 || !json.Valid(value.Payload) {
		return errors.New("invalid_event")
	}
	if header := strings.TrimSpace(request.Header.Get(outbox.WebhookEventIDHeader)); header != value.ID {
		return errors.New("event_id_mismatch")
	}
	if header := strings.TrimSpace(request.Header.Get("Idempotency-Key")); header != value.ID {
		return errors.New("idempotency_key_mismatch")
	}
	if header := strings.TrimSpace(request.Header.Get("X-AI-Runtime-Event-Topic")); header != value.Topic {
		return errors.New("event_topic_mismatch")
	}
	return nil
}

func (h *Handler) record(value Receipt) (duplicate, conflict bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneLocked(value.ReceivedAt)
	if existing, exists := h.receipts[value.ID]; exists {
		return existing.Digest == value.Digest, existing.Digest != value.Digest
	}
	if len(h.receipts) >= h.config.MaximumReceipts {
		h.evictOldestLocked()
	}
	h.receipts[value.ID] = value
	return false, false
}

func (h *Handler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneLocked(h.now())
	return len(h.receipts)
}

func (h *Handler) pruneLocked(now time.Time) {
	cutoff := now.Add(-h.config.Retention)
	for id, value := range h.receipts {
		if value.ReceivedAt.Before(cutoff) {
			delete(h.receipts, id)
		}
	}
}

func (h *Handler) evictOldestLocked() {
	var oldestID string
	var oldest time.Time
	for id, value := range h.receipts {
		if oldestID == "" || value.ReceivedAt.Before(oldest) || (value.ReceivedAt.Equal(oldest) && id < oldestID) {
			oldestID = id
			oldest = value.ReceivedAt
		}
	}
	if oldestID != "" {
		delete(h.receipts, oldestID)
	}
}

func (h *Handler) now() time.Time { return h.config.Now().UTC() }

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

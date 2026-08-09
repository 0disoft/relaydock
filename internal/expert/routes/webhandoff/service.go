package webhandoff

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/expert/resultcontract"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
)

type Handoff struct {
	ConsultationID, ReadToken string
	ExpiresAt                 time.Time
}
type record struct {
	ConsultationID, TokenHash string
	ExpiresAt                 time.Time
	Result                    *resultcontract.Result
}
type Service struct {
	mu      sync.Mutex
	records map[string]record
	ttl     time.Duration
}

func NewService(ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Service{records: map[string]record{}, ttl: ttl}
}
func (s *Service) Create(ctx context.Context, consultationID string) (Handoff, error) {
	if err := ctx.Err(); err != nil {
		return Handoff{}, err
	}
	if consultationID == "" {
		return Handoff{}, core.ErrInvalidArgument
	}
	token := idgen.New("read")
	h := Handoff{ConsultationID: consultationID, ReadToken: token, ExpiresAt: time.Now().UTC().Add(s.ttl)}
	s.mu.Lock()
	s.records[consultationID] = record{ConsultationID: consultationID, TokenHash: hash(token), ExpiresAt: h.ExpiresAt}
	s.mu.Unlock()
	return h, nil
}
func (s *Service) Resolve(ctx context.Context, token string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	needle := hash(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, r := range s.records {
		if time.Now().After(r.ExpiresAt) {
			delete(s.records, id)
			continue
		}
		if r.TokenHash == needle {
			return id, nil
		}
	}
	return "", core.ErrUnauthorized
}
func (s *Service) ImportResult(ctx context.Context, consultationID string, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var result resultcontract.Result
	if err := json.Unmarshal(payload, &result); err != nil {
		return err
	}
	if err := resultcontract.Validate(result); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[consultationID]
	if !ok {
		return core.ErrNotFound
	}
	if time.Now().After(r.ExpiresAt) {
		delete(s.records, consultationID)
		return core.ErrNotFound
	}
	r.Result = &result
	s.records[consultationID] = r
	return nil
}
func hash(v string) string { sum := sha256.Sum256([]byte(v)); return hex.EncodeToString(sum[:]) }

package virtualkey

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"sync"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/idgen"
)

type Record struct {
	Environment, PublicID, VirtualKeyID, TenantID, ProjectID string
	SecretMAC                                                []byte
	Scopes, AllowedModels                                    []string
	ExpiresAt                                                time.Time
	Disabled                                                 bool
}
type MemoryAuthenticator struct {
	mu      sync.RWMutex
	pepper  []byte
	records map[string]Record
}

func NewMemoryAuthenticator(pepper []byte) *MemoryAuthenticator {
	if len(pepper) == 0 {
		pepper = make([]byte, 32)
		_, _ = rand.Read(pepper)
	}
	return &MemoryAuthenticator{pepper: append([]byte(nil), pepper...), records: map[string]Record{}}
}
func (a *MemoryAuthenticator) Issue(environment, tenant, project string, scopes, models []string, expires time.Time) (string, Record, error) {
	if environment == "" {
		environment = "live"
	}
	publicID := idgen.New("vk")
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", Record{}, err
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	record := Record{Environment: environment, PublicID: publicID, VirtualKeyID: idgen.New("key"), TenantID: tenant, ProjectID: project, SecretMAC: a.mac(secret), Scopes: append([]string(nil), scopes...), AllowedModels: append([]string(nil), models...), ExpiresAt: expires}
	a.mu.Lock()
	a.records[publicID] = record
	a.mu.Unlock()
	return "zg_" + environment + "_" + publicID + "." + secret, record, nil
}
func (a *MemoryAuthenticator) Authenticate(ctx context.Context, raw string) (Principal, error) {
	if err := ctx.Err(); err != nil {
		return Principal{}, err
	}
	presented, err := Parse(raw)
	if err != nil {
		return Principal{}, err
	}
	a.mu.RLock()
	record, ok := a.records[presented.PublicID]
	a.mu.RUnlock()
	if !ok || record.Disabled || record.Environment != presented.Environment {
		return Principal{}, core.ErrUnauthorized
	}
	if !record.ExpiresAt.IsZero() && time.Now().After(record.ExpiresAt) {
		return Principal{}, core.ErrUnauthorized
	}
	actual := a.mac(presented.Secret)
	if subtle.ConstantTimeCompare(actual, record.SecretMAC) != 1 {
		return Principal{}, core.ErrUnauthorized
	}
	return Principal{TenantID: record.TenantID, ProjectID: record.ProjectID, VirtualKeyID: record.VirtualKeyID, Scopes: append([]string(nil), record.Scopes...), AllowedModels: append([]string(nil), record.AllowedModels...)}, nil
}
func (a *MemoryAuthenticator) mac(secret string) []byte {
	h := hmac.New(sha256.New, a.pepper)
	_, _ = h.Write([]byte(secret))
	return h.Sum(nil)
}

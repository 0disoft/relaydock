package scopedtoken

import (
	"context"
	"time"
)

const (
	legacyTokenVersion       = "argt1"
	tokenVersion             = "argt2"
	maximumTokenPayloadBytes = 64 << 10
	maximumKeyCount          = 64
	maximumSecretBytes       = 4 << 10
	maximumSubjectBytes      = 512
	maximumIdentityBytes     = 256
	maximumScopeCount        = 64
	maximumScopeBytes        = 256
)

type Claims struct {
	Subject   string   `json:"sub"`
	TenantID  string   `json:"tenantId,omitempty"`
	ProjectID string   `json:"projectId,omitempty"`
	Audience  string   `json:"aud"`
	Scopes    []string `json:"scopes"`
	TokenID   string   `json:"jti"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
}

type Service struct {
	activeKeyID     string
	keys            map[string][]byte
	legacySecrets   [][]byte
	allowLegacy     bool
	audience        string
	maximumLifetime time.Duration
	now             func() time.Time
}

type Verifier interface {
	Verify(context.Context, string) (Claims, error)
}

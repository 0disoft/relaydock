package virtualkey

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
)

type PostgresAuthenticator struct {
	db          *sql.DB
	pepper      []byte
	environment string
}

func NewPostgresAuthenticator(db *sql.DB, pepper []byte, environment string) (*PostgresAuthenticator, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: virtual key database", core.ErrInvalidConfiguration)
	}
	if len(pepper) < 32 {
		return nil, fmt.Errorf("%w: virtual key pepper must be at least 32 bytes", core.ErrInvalidConfiguration)
	}
	environment = strings.TrimSpace(environment)
	if environment == "" {
		environment = "live"
	}
	if strings.ContainsAny(environment, "_.") {
		return nil, fmt.Errorf("%w: virtual key environment", core.ErrInvalidConfiguration)
	}
	return &PostgresAuthenticator{db: db, pepper: append([]byte(nil), pepper...), environment: environment}, nil
}

func (a *PostgresAuthenticator) Authenticate(ctx context.Context, raw string) (Principal, error) {
	if err := ctx.Err(); err != nil {
		return Principal{}, err
	}
	presented, err := Parse(raw)
	if err != nil || presented.Environment != a.environment {
		return Principal{}, core.ErrUnauthorized
	}
	var principal Principal
	var storedMAC []byte
	var expiresAt sql.NullTime
	var revokedAt sql.NullTime
	if err := a.db.QueryRowContext(ctx, `
SELECT
    virtual_key.id::text,
    organization.id::text,
    project.id::text,
    virtual_key.secret_digest,
    virtual_key.scopes,
    virtual_key.allowed_models,
    virtual_key.expires_at,
    virtual_key.revoked_at
FROM control.virtual_keys AS virtual_key
JOIN control.projects AS project ON project.id = virtual_key.project_id
JOIN control.organizations AS organization ON organization.id = project.organization_id
WHERE virtual_key.public_id = $1`, presented.PublicID).Scan(
		&principal.VirtualKeyID,
		&principal.TenantID,
		&principal.ProjectID,
		&storedMAC,
		&principal.Scopes,
		&principal.AllowedModels,
		&expiresAt,
		&revokedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return Principal{}, core.ErrUnauthorized
		}
		return Principal{}, fmt.Errorf("authenticate virtual key: %w", err)
	}
	if revokedAt.Valid || (expiresAt.Valid && !expiresAt.Time.After(time.Now().UTC())) {
		return Principal{}, core.ErrUnauthorized
	}
	actual := a.mac(presented.Secret)
	if len(storedMAC) != len(actual) || subtle.ConstantTimeCompare(storedMAC, actual) != 1 {
		return Principal{}, core.ErrUnauthorized
	}
	principal.Scopes = append([]string(nil), principal.Scopes...)
	principal.AllowedModels = append([]string(nil), principal.AllowedModels...)
	return principal, nil
}

// Issue creates a key once and returns the plaintext secret only to the
// caller. PostgreSQL stores only the peppered HMAC. tenantID may be empty; if
// set, it must match the owning organization ID of projectID.
func (a *PostgresAuthenticator) Issue(ctx context.Context, tenantID, projectID string, scopes, models []string, expiresAt time.Time) (string, Record, error) {
	if err := ctx.Err(); err != nil {
		return "", Record{}, err
	}
	tenantID = strings.TrimSpace(tenantID)
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return "", Record{}, fmt.Errorf("%w: project ID", core.ErrInvalidArgument)
	}
	if !expiresAt.IsZero() {
		expiresAt = expiresAt.UTC()
		if !expiresAt.After(time.Now().UTC()) {
			return "", Record{}, fmt.Errorf("%w: virtual key expiry must be in the future", core.ErrInvalidArgument)
		}
	}

	var actualTenantID string
	if err := a.db.QueryRowContext(ctx, `
SELECT organization.id::text
FROM control.projects AS project
JOIN control.organizations AS organization ON organization.id = project.organization_id
WHERE project.id = $1::uuid`, projectID).Scan(&actualTenantID); err != nil {
		if err == sql.ErrNoRows {
			return "", Record{}, core.ErrNotFound
		}
		return "", Record{}, fmt.Errorf("resolve virtual key project: %w", err)
	}
	if tenantID != "" && tenantID != actualTenantID {
		return "", Record{}, core.ErrForbidden
	}

	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", Record{}, fmt.Errorf("generate virtual key secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	publicID := idgen.New("vk")
	record := Record{
		Environment:   a.environment,
		PublicID:      publicID,
		TenantID:      actualTenantID,
		ProjectID:     projectID,
		SecretMAC:     a.mac(secret),
		Scopes:        normalizedValues(scopes),
		AllowedModels: normalizedValues(models),
		ExpiresAt:     expiresAt,
	}
	var virtualKeyID string
	if err := a.db.QueryRowContext(ctx, `
INSERT INTO control.virtual_keys (
    project_id, public_id, secret_digest, scopes, allowed_models, expires_at
) VALUES ($1::uuid, $2, $3, $4, $5, $6)
RETURNING id::text`,
		projectID, publicID, record.SecretMAC, record.Scopes, record.AllowedModels, nullableTime(expiresAt),
	).Scan(&virtualKeyID); err != nil {
		return "", Record{}, fmt.Errorf("issue virtual key: %w", err)
	}
	record.VirtualKeyID = virtualKeyID
	return "zg_" + a.environment + "_" + publicID + "." + secret, record, nil
}

func (a *PostgresAuthenticator) Revoke(ctx context.Context, virtualKeyID, projectID string) error {
	virtualKeyID = strings.TrimSpace(virtualKeyID)
	projectID = strings.TrimSpace(projectID)
	if virtualKeyID == "" || projectID == "" {
		return core.ErrInvalidArgument
	}
	result, err := a.db.ExecContext(ctx, `
UPDATE control.virtual_keys
SET revoked_at = COALESCE(revoked_at, now())
WHERE id = $1::uuid AND project_id = $2::uuid`, virtualKeyID, projectID)
	if err != nil {
		return fmt.Errorf("revoke virtual key: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (a *PostgresAuthenticator) mac(secret string) []byte {
	hash := hmac.New(sha256.New, a.pepper)
	_, _ = hash.Write([]byte(secret))
	return hash.Sum(nil)
}

func normalizedValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

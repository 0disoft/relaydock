package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/auth/mcpscope"
	"github.com/0disoft/relaydock/internal/auth/scopedtoken"
)

type output struct {
	Token     string             `json:"token"`
	KeyID     string             `json:"keyId"`
	ExpiresAt time.Time          `json:"expiresAt"`
	Claims    scopedtoken.Claims `json:"claims"`
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "issue" {
		fmt.Fprintln(os.Stderr, "usage: mcptokenctl issue --subject <name> --scopes <scope,scope> [--ttl 1h] [--tenant id] [--project id] [--raw]")
		os.Exit(2)
	}
	flags := flag.NewFlagSet("issue", flag.ExitOnError)
	subject := flags.String("subject", "", "token subject")
	scopes := flags.String("scopes", "consultations:read", "comma-separated scopes")
	tenant := flags.String("tenant", "", "tenant identifier")
	project := flags.String("project", "", "project identifier")
	ttl := flags.Duration("ttl", time.Hour, "token lifetime")
	rawOutput := flags.Bool("raw", false, "print only the token")
	_ = flags.Parse(os.Args[2:])

	secret, err := tokenSecret()
	if err != nil {
		fail(err)
	}
	keyID := strings.TrimSpace(os.Getenv("EXPERT_MCP_TOKEN_KEY_ID"))
	if keyID == "" {
		keyID = scopedtoken.KeyID(secret)
	}
	service, err := scopedtoken.NewKeyRing(keyID, secret, nil, envOr("EXPERT_MCP_TOKEN_AUDIENCE", "expert-mcp"))
	if err != nil {
		fail(err)
	}
	maximumLifetime, err := time.ParseDuration(envOr("EXPERT_MCP_TOKEN_MAX_LIFETIME", "24h"))
	if err != nil || maximumLifetime <= 0 {
		fail(fmt.Errorf("EXPERT_MCP_TOKEN_MAX_LIFETIME must be a positive duration"))
	}
	service = service.WithMaximumLifetime(maximumLifetime)
	requestedScopes := splitCSV(*scopes)
	if err := mcpscope.ValidateIssuedClaims(requestedScopes, *tenant, *project); err != nil {
		fail(err)
	}
	normalizedScopes, err := mcpscope.Normalize(requestedScopes)
	if err != nil {
		fail(err)
	}
	token, claims, err := service.Issue(context.Background(), scopedtoken.Claims{
		Subject:   strings.TrimSpace(*subject),
		TenantID:  strings.TrimSpace(*tenant),
		ProjectID: strings.TrimSpace(*project),
		Scopes:    normalizedScopes,
	}, *ttl)
	if err != nil {
		fail(err)
	}
	if *rawOutput {
		fmt.Println(token)
		return
	}
	encoded, err := json.MarshalIndent(output{
		Token:     token,
		KeyID:     service.ActiveKeyID(),
		ExpiresAt: time.Unix(claims.ExpiresAt, 0).UTC(),
		Claims:    claims,
	}, "", "  ")
	if err != nil {
		fail(err)
	}
	fmt.Println(string(encoded))
}

func tokenSecret() ([]byte, error) {
	if encoded := strings.TrimSpace(os.Getenv("EXPERT_MCP_TOKEN_SECRET_B64")); encoded != "" {
		secret, err := scopedtoken.DecodeBase64Secret(encoded)
		if err != nil {
			return nil, fmt.Errorf("decode EXPERT_MCP_TOKEN_SECRET_B64")
		}
		return secret, nil
	}
	if raw := os.Getenv("EXPERT_MCP_TOKEN_SECRET"); raw != "" {
		return []byte(raw), nil
	}
	return nil, fmt.Errorf("EXPERT_MCP_TOKEN_SECRET or EXPERT_MCP_TOKEN_SECRET_B64 is required")
}

func splitCSV(value string) []string {
	out := make([]string, 0)
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mcptokenctl:", err)
	os.Exit(1)
}

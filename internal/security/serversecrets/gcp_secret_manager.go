package serversecrets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

const (
	gcpMetadataTokenURL = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"
	gcpSecretAPIBase    = "https://secretmanager.googleapis.com/v1/"
	maximumTokenBody    = 64 << 10
	maximumSecretBody   = 128 << 10
	maximumSecretValue  = 64 << 10
)

type GCPSecretManager struct {
	metadataClient   *http.Client
	apiClient        *http.Client
	metadataTokenURL string
	secretAPIBase    string
}

func NewGCPSecretManager() *GCPSecretManager {
	metadataTransport := http.DefaultTransport.(*http.Transport).Clone()
	metadataTransport.Proxy = nil
	return newGCPSecretManagerWithTransports(metadataTransport, http.DefaultTransport, gcpMetadataTokenURL, gcpSecretAPIBase)
}

func newGCPSecretManager(transport http.RoundTripper, metadataTokenURL, secretAPIBase string) *GCPSecretManager {
	return newGCPSecretManagerWithTransports(transport, transport, metadataTokenURL, secretAPIBase)
}

func newGCPSecretManagerWithTransports(metadataTransport, apiTransport http.RoundTripper, metadataTokenURL, secretAPIBase string) *GCPSecretManager {
	if metadataTransport == nil {
		metadataTransport = http.DefaultTransport
	}
	if apiTransport == nil {
		apiTransport = http.DefaultTransport
	}
	newClient := func(transport http.RoundTripper) *http.Client {
		return &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &GCPSecretManager{
		metadataClient:   newClient(metadataTransport),
		apiClient:        newClient(apiTransport),
		metadataTokenURL: metadataTokenURL,
		secretAPIBase:    secretAPIBase,
	}
}

func (*GCPSecretManager) Scheme() string { return "gcp-sm" }

func (g *GCPSecretManager) Resolve(ctx context.Context, resource string) ([]byte, error) {
	canonicalResource, err := canonicalGCPSecretResource(resource)
	if err != nil {
		return nil, err
	}
	token, err := g.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	defer zeroBytes(token)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, g.secretAPIBase+canonicalResource+":access", nil)
	if err != nil {
		return nil, fmt.Errorf("build GCP Secret Manager request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+string(token))
	response, err := g.apiClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request GCP Secret Manager: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GCP Secret Manager returned HTTP %d", response.StatusCode)
	}
	body, err := readBounded(response.Body, maximumSecretBody)
	if err != nil {
		return nil, fmt.Errorf("read GCP Secret Manager response: %w", err)
	}
	defer zeroBytes(body)
	var document struct {
		Payload struct {
			Data       string `json:"data"`
			DataCRC32C string `json:"dataCrc32c"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, fmt.Errorf("decode GCP Secret Manager response: %w", err)
	}
	if document.Payload.Data == "" || document.Payload.DataCRC32C == "" {
		return nil, fmt.Errorf("%w: GCP Secret Manager response omitted payload integrity data", core.ErrCorruptState)
	}
	value, err := base64.StdEncoding.DecodeString(document.Payload.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: decode GCP Secret Manager payload", core.ErrCorruptState)
	}
	if len(value) == 0 || len(value) > maximumSecretValue {
		zeroBytes(value)
		return nil, fmt.Errorf("%w: GCP Secret Manager payload size is outside the supported range", core.ErrInvalidConfiguration)
	}
	wantCRC, err := strconv.ParseUint(document.Payload.DataCRC32C, 10, 32)
	if err != nil || uint32(wantCRC) != crc32.Checksum(value, crc32.MakeTable(crc32.Castagnoli)) {
		zeroBytes(value)
		return nil, fmt.Errorf("%w: GCP Secret Manager payload checksum mismatch", core.ErrCorruptState)
	}
	return value, nil
}

func (g *GCPSecretManager) accessToken(ctx context.Context) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, g.metadataTokenURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build GCP metadata request: %w", err)
	}
	request.Header.Set("Metadata-Flavor", "Google")
	response, err := g.metadataClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request GCP workload identity token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GCP metadata server returned HTTP %d", response.StatusCode)
	}
	if !strings.EqualFold(strings.TrimSpace(response.Header.Get("Metadata-Flavor")), "Google") {
		return nil, fmt.Errorf("%w: GCP metadata response lacks the required Metadata-Flavor header", core.ErrUnauthorized)
	}
	body, err := readBounded(response.Body, maximumTokenBody)
	if err != nil {
		return nil, fmt.Errorf("read GCP workload identity token: %w", err)
	}
	defer zeroBytes(body)
	var document struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, fmt.Errorf("decode GCP workload identity token: %w", err)
	}
	if document.AccessToken == "" || len(document.AccessToken) > 16<<10 || !strings.EqualFold(document.TokenType, "Bearer") || document.ExpiresIn <= 0 {
		return nil, fmt.Errorf("%w: invalid GCP workload identity token response", core.ErrUnauthorized)
	}
	return []byte(document.AccessToken), nil
}

func canonicalGCPSecretResource(resource string) (string, error) {
	segments := strings.Split(resource, "/")
	if len(segments) != 6 || segments[0] != "projects" || segments[2] != "secrets" || segments[4] != "versions" {
		return "", fmt.Errorf("%w: GCP secret reference must be projects/PROJECT/secrets/SECRET/versions/VERSION", core.ErrInvalidConfiguration)
	}
	for _, index := range []int{1, 3, 5} {
		segment := segments[index]
		if !validGCPResourceSegment(segment, index == 1) {
			return "", fmt.Errorf("%w: invalid GCP secret resource segment", core.ErrInvalidConfiguration)
		}
		segments[index] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/"), nil
}

func validGCPResourceSegment(segment string, project bool) bool {
	if segment == "" || len(segment) > 255 {
		return false
	}
	for _, char := range segment {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' {
			continue
		}
		if !project && char == '_' {
			continue
		}
		return false
	}
	return true
}

func readBounded(reader io.Reader, maximum int64) ([]byte, error) {
	value, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(value)) > maximum {
		return nil, fmt.Errorf("response exceeds %d bytes", maximum)
	}
	return value, nil
}

func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

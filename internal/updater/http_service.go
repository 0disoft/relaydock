package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

type Manifest struct {
	Version      string `json:"version"`
	ReleaseNotes string `json:"releaseNotes,omitempty"`
	ArtifactURL  string `json:"artifactUrl"`
	SHA256       string `json:"sha256"`
	Size         int64  `json:"size"`
	Signature    string `json:"signature,omitempty"`
}

type HTTPServiceOptions struct {
	ManifestURL       string
	DownloadDirectory string
	CurrentVersion    string
	PublicKey         ed25519.PublicKey
	RequireSignature  bool
	MaximumManifest   int64
	MaximumArtifact   int64
	Client            *http.Client
}

// HTTPService stages verified update artifacts. InstallOnExit writes a pending
// update marker; a small platform-specific bootstrapper can atomically replace
// the executable after the Wails process exits.
type HTTPService struct {
	options HTTPServiceOptions

	mu       sync.Mutex
	manifest *Manifest
	staged   map[string]string
}

func NewHTTPService(options HTTPServiceOptions) (*HTTPService, error) {
	if strings.TrimSpace(options.ManifestURL) == "" {
		return nil, fmt.Errorf("%w: update manifest URL", core.ErrInvalidConfiguration)
	}
	if _, err := url.ParseRequestURI(options.ManifestURL); err != nil {
		return nil, fmt.Errorf("%w: update manifest URL: %v", core.ErrInvalidConfiguration, err)
	}
	if strings.TrimSpace(options.DownloadDirectory) == "" {
		return nil, fmt.Errorf("%w: update download directory", core.ErrInvalidConfiguration)
	}
	if options.RequireSignature && len(options.PublicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: Ed25519 update public key", core.ErrInvalidConfiguration)
	}
	if options.MaximumManifest <= 0 {
		options.MaximumManifest = 1 << 20
	}
	if options.MaximumArtifact <= 0 {
		options.MaximumArtifact = 512 << 20
	}
	if options.Client == nil {
		options.Client = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPService{options: options, staged: make(map[string]string)}, nil
}

func (s *HTTPService) Check(ctx context.Context) (Info, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.options.ManifestURL, nil)
	if err != nil {
		return Info{}, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := s.options.Client.Do(request)
	if err != nil {
		return Info{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return Info{}, fmt.Errorf("update manifest returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	payload, err := readBounded(response.Body, s.options.MaximumManifest)
	if err != nil {
		return Info{}, err
	}
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Info{}, fmt.Errorf("%w: malformed update manifest: %v", core.ErrInvalidArgument, err)
	}
	if err := s.validateManifest(manifest); err != nil {
		return Info{}, err
	}
	s.mu.Lock()
	s.manifest = &manifest
	s.mu.Unlock()
	return Info{
		Version:      manifest.Version,
		ReleaseNotes: manifest.ReleaseNotes,
		DownloadSize: manifest.Size,
		Available:    manifest.Version != "" && manifest.Version != s.options.CurrentVersion,
		SHA256:       strings.ToLower(manifest.SHA256),
	}, nil
}

func (s *HTTPService) Download(ctx context.Context, version string) error {
	manifest, err := s.cachedManifest(version)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manifest.ArtifactURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/octet-stream")
	response, err := s.options.Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("update artifact returned %d", response.StatusCode)
	}
	maximum := s.options.MaximumArtifact
	if manifest.Size > 0 && manifest.Size < maximum {
		maximum = manifest.Size
	}
	if err := os.MkdirAll(s.options.DownloadDirectory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(s.options.DownloadDirectory, ".update-*.partial")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, maximum+1))
	if err != nil {
		return err
	}
	if written > maximum || (manifest.Size > 0 && written != manifest.Size) {
		return core.ErrFrameTooLarge
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, manifest.SHA256) {
		return fmt.Errorf("%w: update artifact digest mismatch", core.ErrUnauthorized)
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	finalPath := filepath.Join(s.options.DownloadDirectory, safeVersionFilename(version)+".artifact")
	if err := os.Rename(temporaryName, finalPath); err != nil {
		return err
	}
	committed = true
	s.mu.Lock()
	s.staged[version] = finalPath
	s.mu.Unlock()
	return nil
}

func (s *HTTPService) InstallOnExit(ctx context.Context, version string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	artifact := s.staged[version]
	s.mu.Unlock()
	if artifact == "" {
		return fmt.Errorf("%w: update version %s is not staged", core.ErrConflict, version)
	}
	marker := struct {
		Version      string    `json:"version"`
		ArtifactPath string    `json:"artifactPath"`
		CreatedAt    time.Time `json:"createdAt"`
	}{Version: version, ArtifactPath: artifact, CreatedAt: time.Now().UTC()}
	payload, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	temporary := filepath.Join(s.options.DownloadDirectory, ".pending-update.json.tmp")
	final := filepath.Join(s.options.DownloadDirectory, "pending-update.json")
	if err := os.WriteFile(temporary, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, final)
}

func (s *HTTPService) cachedManifest(version string) (Manifest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.manifest == nil || s.manifest.Version != strings.TrimSpace(version) {
		return Manifest{}, fmt.Errorf("%w: call Check before downloading version %s", core.ErrConflict, version)
	}
	return *s.manifest, nil
}

func (s *HTTPService) validateManifest(manifest Manifest) error {
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.ArtifactURL = strings.TrimSpace(manifest.ArtifactURL)
	manifest.SHA256 = strings.ToLower(strings.TrimSpace(manifest.SHA256))
	if manifest.Version == "" || manifest.ArtifactURL == "" || len(manifest.SHA256) != sha256.Size*2 || manifest.Size <= 0 {
		return fmt.Errorf("%w: incomplete update manifest", core.ErrInvalidArgument)
	}
	if _, err := hex.DecodeString(manifest.SHA256); err != nil {
		return fmt.Errorf("%w: invalid update digest", core.ErrInvalidArgument)
	}
	artifactURL, err := url.Parse(manifest.ArtifactURL)
	if err != nil || artifactURL.Scheme != "https" && artifactURL.Scheme != "http" {
		return fmt.Errorf("%w: invalid update artifact URL", core.ErrInvalidArgument)
	}
	if manifest.Size > s.options.MaximumArtifact {
		return core.ErrFrameTooLarge
	}
	if s.options.RequireSignature || manifest.Signature != "" {
		if len(s.options.PublicKey) != ed25519.PublicKeySize {
			return fmt.Errorf("%w: update signing key", core.ErrInvalidConfiguration)
		}
		signature, err := base64.RawStdEncoding.DecodeString(manifest.Signature)
		if err != nil || len(signature) != ed25519.SignatureSize {
			return core.ErrUnauthorized
		}
		if !ed25519.Verify(s.options.PublicKey, manifestSigningPayload(manifest), signature) {
			return core.ErrUnauthorized
		}
	}
	return nil
}

func manifestSigningPayload(manifest Manifest) []byte {
	return []byte(strings.Join([]string{
		strings.TrimSpace(manifest.Version),
		strings.TrimSpace(manifest.ArtifactURL),
		strings.ToLower(strings.TrimSpace(manifest.SHA256)),
		fmt.Sprintf("%d", manifest.Size),
	}, "\n"))
}

func readBounded(reader io.Reader, maximum int64) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > maximum {
		return nil, core.ErrFrameTooLarge
	}
	return payload, nil
}

func safeVersionFilename(version string) string {
	version = strings.TrimSpace(version)
	var builder strings.Builder
	for _, r := range version {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "update"
	}
	return builder.String()
}

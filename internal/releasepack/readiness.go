package releasepack

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const publicModulePath = "github.com/0disoft/relaydock"

const (
	releaseSignatureSchema   = "relaydock.release-checksums-signature/v1"
	releaseSignatureDomain   = "relaydock.release-checksums/v1"
	maximumChecksumsFileSize = 1 << 20
)

type ReleaseSignature struct {
	SchemaVersion string `json:"schemaVersion"`
	Algorithm     string `json:"algorithm"`
	KeyID         string `json:"keyId"`
	SubjectSHA256 string `json:"subjectSha256"`
	Signature     string `json:"signature"`
}

type ReadinessReport struct {
	Version string
	Module  string
	Files   int
}

func CheckReleaseReadiness(root, expectedVersion string) (ReadinessReport, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return ReadinessReport{}, err
	}
	expectedVersion = strings.TrimSpace(expectedVersion)
	var blockers []string
	version := readTrimmed(filepath.Join(root, "VERSION"), "VERSION", &blockers)
	if expectedVersion == "" {
		blockers = append(blockers, "expected release version is required")
	} else if version != "" && version != expectedVersion {
		blockers = append(blockers, fmt.Sprintf("VERSION is %q, expected %q", version, expectedVersion))
	}
	module := readTrimmed(filepath.Join(root, "go.mod"), "go.mod", &blockers)
	if module != "" && !strings.HasPrefix(module, "module "+publicModulePath+"\n") && module != "module "+publicModulePath {
		blockers = append(blockers, "go.mod does not declare "+publicModulePath)
	}
	for _, required := range []string{"go.sum", "bun.lock", "LICENSE", "NOTICE"} {
		readTrimmed(filepath.Join(root, required), required, &blockers)
	}
	if _, err := os.Stat(filepath.Join(root, "LICENSE-PENDING.md")); err == nil {
		blockers = append(blockers, "LICENSE-PENDING.md must be resolved before release")
	} else if !os.IsNotExist(err) {
		blockers = append(blockers, fmt.Sprintf("inspect LICENSE-PENDING.md: %v", err))
	}
	manifest, manifestErr := Verify(root)
	if manifestErr != nil {
		blockers = append(blockers, "source manifest is not current: "+manifestErr.Error())
	}
	if len(blockers) > 0 {
		return ReadinessReport{}, fmt.Errorf("release readiness failed: %s", strings.Join(blockers, "; "))
	}
	return ReadinessReport{Version: version, Module: publicModulePath, Files: manifest.Files}, nil
}

func readTrimmed(path, label string, blockers *[]string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		*blockers = append(*blockers, fmt.Sprintf("%s is required: %v", label, err))
		return ""
	}
	value := strings.TrimSpace(string(raw))
	if value == "" {
		*blockers = append(*blockers, label+" must not be empty")
	}
	return value
}

func DecodeReleasePrivateKey(encoded string) (ed25519.PrivateKey, error) {
	decoded, err := decodeReleaseKey(encoded)
	if err != nil {
		return nil, err
	}
	switch len(decoded) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(decoded), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(decoded), nil
	default:
		return nil, fmt.Errorf("release signing key must decode to an Ed25519 seed or private key")
	}
}

func DecodeReleasePublicKey(encoded string) (ed25519.PublicKey, error) {
	decoded, err := decodeReleaseKey(encoded)
	if err != nil {
		return nil, err
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("release verification key must decode to an Ed25519 public key")
	}
	return ed25519.PublicKey(decoded), nil
}

func SignReleaseChecksums(path string, privateKey ed25519.PrivateKey) (ReleaseSignature, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return ReleaseSignature{}, fmt.Errorf("invalid Ed25519 release private key")
	}
	digest, err := releaseChecksumsDigest(path)
	if err != nil {
		return ReleaseSignature{}, err
	}
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return ReleaseSignature{}, fmt.Errorf("derive Ed25519 release public key")
	}
	signature := ed25519.Sign(privateKey, releaseSignaturePayload(digest))
	return ReleaseSignature{
		SchemaVersion: releaseSignatureSchema,
		Algorithm:     "Ed25519",
		KeyID:         releaseSigningKeyID(publicKey),
		SubjectSHA256: digest,
		Signature:     base64.RawStdEncoding.EncodeToString(signature),
	}, nil
}

func VerifyReleaseChecksums(path string, envelope ReleaseSignature, publicKey ed25519.PublicKey) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid Ed25519 release public key")
	}
	if envelope.SchemaVersion != releaseSignatureSchema || envelope.Algorithm != "Ed25519" {
		return fmt.Errorf("unsupported release signature contract")
	}
	if envelope.KeyID != releaseSigningKeyID(publicKey) {
		return fmt.Errorf("release signature key ID does not match public key")
	}
	if len(envelope.SubjectSHA256) != sha256.Size*2 {
		return fmt.Errorf("invalid release signature subject digest")
	}
	if _, err := hex.DecodeString(envelope.SubjectSHA256); err != nil {
		return fmt.Errorf("invalid release signature subject digest: %w", err)
	}
	digest, err := releaseChecksumsDigest(path)
	if err != nil {
		return err
	}
	if digest != envelope.SubjectSHA256 {
		return fmt.Errorf("release checksum signature subject changed")
	}
	signature, err := base64.RawStdEncoding.DecodeString(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid Ed25519 release signature")
	}
	if !ed25519.Verify(publicKey, releaseSignaturePayload(digest), signature) {
		return fmt.Errorf("verify Ed25519 release signature")
	}
	return nil
}

func ReadReleaseSignature(path string) (ReleaseSignature, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ReleaseSignature{}, fmt.Errorf("read release signature: %w", err)
	}
	var envelope ReleaseSignature
	if err := decodeStrictJSON(raw, &envelope, "release signature"); err != nil {
		return ReleaseSignature{}, err
	}
	return envelope, nil
}

func WriteReleaseSignature(path string, envelope ReleaseSignature) error {
	payload, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return fmt.Errorf("encode release signature: %w", err)
	}
	payload = append(payload, '\n')
	directory := filepath.Dir(filepath.Clean(path))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create release signature directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".release-signature-*.tmp")
	if err != nil {
		return fmt.Errorf("create release signature: %w", err)
	}
	temporaryPath := temporary.Name()
	published := false
	defer func() {
		_ = temporary.Close()
		if !published {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		return err
	}
	published = true
	return nil
}

func releaseChecksumsDigest(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("inspect release checksums: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximumChecksumsFileSize {
		return "", fmt.Errorf("release checksums must be a non-empty regular file no larger than %d bytes", maximumChecksumsFileSize)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read release checksums: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func releaseSignaturePayload(digest string) []byte {
	return []byte(releaseSignatureDomain + "\n" + digest + "\n")
}

func releaseSigningKeyID(publicKey ed25519.PublicKey) string {
	digest := sha256.Sum256(publicKey)
	return hex.EncodeToString(digest[:16])
}

func decodeReleaseKey(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, fmt.Errorf("release signing key is required")
	}
	encodings := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	}
	for _, encoding := range encodings {
		if decoded, err := encoding.DecodeString(encoded); err == nil {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("release signing key must be base64 or base64url")
}

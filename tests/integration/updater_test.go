package integration_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/your-org/ai-runtime-gateway/internal/updater"
)

func TestUpdaterStagesOnlySignedAndHashedArtifact(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	artifact := []byte("signed desktop release artifact")
	digest := sha256.Sum256(artifact)
	digestHex := hex.EncodeToString(digest[:])
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artifact":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(artifact)
		case "/manifest":
			artifactURL := server.URL + "/artifact"
			payload := strings.Join([]string{"0.2.0", artifactURL, digestHex, fmt.Sprintf("%d", len(artifact))}, "\n")
			manifest := updater.Manifest{
				Version:      "0.2.0",
				ReleaseNotes: "verified test release",
				ArtifactURL:  artifactURL,
				SHA256:       digestHex,
				Size:         int64(len(artifact)),
				Signature:    base64.RawStdEncoding.EncodeToString(ed25519.Sign(private, []byte(payload))),
			}
			_ = json.NewEncoder(w).Encode(manifest)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	directory := t.TempDir()
	service, err := updater.NewHTTPService(updater.HTTPServiceOptions{
		ManifestURL:       server.URL + "/manifest",
		DownloadDirectory: directory,
		CurrentVersion:    "0.1.0",
		PublicKey:         public,
		RequireSignature:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := service.Check(context.Background())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !info.Available || info.Version != "0.2.0" || info.SHA256 != digestHex {
		t.Fatalf("unexpected update info: %+v", info)
	}
	if err := service.Download(context.Background(), info.Version); err != nil {
		t.Fatalf("download: %v", err)
	}
	if err := service.InstallOnExit(context.Background(), info.Version); err != nil {
		t.Fatalf("stage install: %v", err)
	}
	marker, err := os.Open(filepath.Join(directory, "pending-update.json"))
	if err != nil {
		t.Fatalf("open pending marker: %v", err)
	}
	defer marker.Close()
	var pending struct {
		Version      string `json:"version"`
		ArtifactPath string `json:"artifactPath"`
	}
	if err := json.NewDecoder(marker).Decode(&pending); err != nil {
		t.Fatal(err)
	}
	if pending.Version != "0.2.0" {
		t.Fatalf("wrong pending version: %+v", pending)
	}
	file, err := os.Open(pending.ArtifactPath)
	if err != nil {
		t.Fatalf("open staged artifact: %v", err)
	}
	defer file.Close()
	staged, _ := io.ReadAll(file)
	if string(staged) != string(artifact) {
		t.Fatalf("staged artifact changed: %q", staged)
	}
}

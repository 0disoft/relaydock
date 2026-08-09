package controlhttp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/auth/controlaccess"
	"github.com/0disoft/relaydock/internal/control/snapshot"
)

const (
	accessTenantA  = "00000000-0000-0000-0000-000000000001"
	accessProjectA = "00000000-0000-0000-0000-000000000002"
	accessProjectB = "00000000-0000-0000-0000-000000000003"
)

func TestProjectViewerSeesOnlyProjectModelsAndCannotReadSnapshot(t *testing.T) {
	t.Parallel()
	api, _ := accessTestAPI(t)
	principal := controlaccess.Principal{Subject: "project-a-viewer", Role: controlaccess.RoleViewer, TenantID: accessTenantA, ProjectID: accessProjectA}

	modelsResponse := serveAs(api.Handler(), principal, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if modelsResponse.Code != http.StatusOK {
		t.Fatalf("models status %d: %s", modelsResponse.Code, modelsResponse.Body.String())
	}
	var body struct {
		Models []snapshot.ModelRoute `json:"models"`
	}
	if err := json.Unmarshal(modelsResponse.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Models) != 1 || body.Models[0].VirtualModel != "code-fast" {
		t.Fatalf("project viewer models = %#v, want code-fast only", body.Models)
	}
	if len(body.Models[0].CandidateDetails) != 1 || body.Models[0].CandidateDetails[0].AccountID != "" {
		t.Fatalf("project viewer received internal provider account identity: %#v", body.Models[0].CandidateDetails)
	}

	snapshotResponse := serveAs(api.Handler(), principal, httptest.NewRequest(http.MethodGet, "/v1/snapshot", nil))
	if snapshotResponse.Code != http.StatusForbidden {
		t.Fatalf("snapshot status %d, want %d", snapshotResponse.Code, http.StatusForbidden)
	}
}

func TestProjectViewerCannotPublishGlobalSnapshot(t *testing.T) {
	t.Parallel()
	api, store := accessTestAPI(t)
	payload := []byte(`{"models":[{"virtualModel":"replacement","candidates":["local/echo"]}]}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/snapshot", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	principal := controlaccess.Principal{Subject: "project-a-viewer", Role: controlaccess.RoleViewer, TenantID: accessTenantA, ProjectID: accessProjectA}
	response := serveAs(api.Handler(), principal, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("publish status %d, want %d", response.Code, http.StatusForbidden)
	}
	current, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 1 {
		t.Fatalf("denied publish changed revision to %d", current.Revision)
	}
}

func TestPublisherCanPublishAndAnonymousRequestIsRejected(t *testing.T) {
	t.Parallel()
	api, store := accessTestAPI(t)
	payload := []byte(`{"models":[{"virtualModel":"replacement","candidates":["local/echo"]}]}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/snapshot", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := serveAs(api.Handler(), controlaccess.Principal{Subject: "publisher-1", Role: controlaccess.RolePublisher}, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("publish status %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	current, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 2 {
		t.Fatalf("published revision %d, want 2", current.Revision)
	}

	anonymous := httptest.NewRecorder()
	api.Handler().ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status %d, want %d", anonymous.Code, http.StatusUnauthorized)
	}
}

func TestDeniedAccessProducesSafeAuditEvent(t *testing.T) {
	t.Parallel()
	api, _ := accessTestAPI(t)
	events := make(chan controlaccess.AuditEvent, 1)
	api.SetAccessAudit(func(_ context.Context, event controlaccess.AuditEvent) { events <- event })
	principal := controlaccess.Principal{Subject: "project-a-viewer", Role: controlaccess.RoleViewer, TenantID: accessTenantA, ProjectID: accessProjectA}
	response := serveAs(api.Handler(), principal, httptest.NewRequest(http.MethodGet, "/v1/snapshot", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("snapshot status %d, want %d", response.Code, http.StatusForbidden)
	}
	event := <-events
	if event.Subject != principal.Subject || event.Action != controlaccess.ActionSnapshotRead || event.Allowed || event.Reason != "permission_denied" {
		t.Fatalf("unexpected audit event: %#v", event)
	}
	if event.PolicyVersion != controlaccess.PolicyVersion {
		t.Fatalf("audit policy version %q, want %q", event.PolicyVersion, controlaccess.PolicyVersion)
	}
}

func accessTestAPI(t *testing.T) (*API, snapshot.Store) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer := snapshot.NewEd25519Signer(private, public)
	store := snapshot.NewMemoryStore()
	value, err := signer.Sign(context.Background(), snapshot.Snapshot{
		Revision: 1, GeneratedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour),
		VirtualKeys: []snapshot.VirtualKey{
			{PublicID: "key-a", TenantID: accessTenantA, ProjectID: accessProjectA, AllowedModels: []string{"code-fast"}},
			{PublicID: "key-b", TenantID: accessTenantA, ProjectID: accessProjectB, AllowedModels: []string{"code-deep"}},
		},
		Models: []snapshot.ModelRoute{
			{VirtualModel: "code-fast", CandidateDetails: []snapshot.RouteCandidate{{Provider: "openai", AccountID: "provider-account-secret", Model: "model-a"}}},
			{VirtualModel: "code-deep", Candidates: []string{"local/echo"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Publish(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	api, err := NewAPIWithDependencies(store, signer, public)
	if err != nil {
		t.Fatal(err)
	}
	return api, store
}

func serveAs(handler http.Handler, principal controlaccess.Principal, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request.WithContext(controlaccess.WithPrincipal(request.Context(), principal)))
	return response
}

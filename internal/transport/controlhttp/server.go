package controlhttp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0disoft/relaydock/internal/auth/controlaccess"
	"github.com/0disoft/relaydock/internal/control/snapshot"
	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/transport/apiutil"
	"github.com/0disoft/relaydock/internal/transport/health"
)

type PublishRequest struct {
	ExpiresInSeconds int64                        `json:"expiresInSeconds,omitempty"`
	VirtualKeys      []snapshot.VirtualKey        `json:"virtualKeys,omitempty"`
	Models           []snapshot.ModelRoute        `json:"models"`
	ProviderRefs     []snapshot.ProviderReference `json:"providerRefs,omitempty"`
	PriceRevisions   []snapshot.PriceRevision     `json:"priceRevisions,omitempty"`
}

type API struct {
	store    snapshot.Store
	signer   snapshot.Signer
	verifier snapshot.Verifier
	public   ed25519.PublicKey
	keyID    string
	audit    controlaccess.AuditFunc
	mu       sync.Mutex
}

func NewHandler() http.Handler {
	api, err := NewAPI()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { apiutil.WriteError(w, err) })
	}
	return api.Handler()
}

func NewAPI() (*API, error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate snapshot signing key: %w", err)
	}
	return NewAPIWithDependencies(snapshot.NewMemoryStore(), snapshot.NewEd25519Signer(private, public), public)
}

// NewAPIWithDependencies builds the control API around a caller-owned store
// and signer. Persisted snapshots are verified before serving. An expired
// snapshot is re-signed as a new revision using the same policy payload so a
// routine restart does not leave the data plane without a valid snapshot.
// Concurrent instances may race to initialize or renew a shared PostgreSQL
// store; a bounded reload loop resolves that race without accepting rollback.
func NewAPIWithDependencies(store snapshot.Store, signer snapshot.Signer, public ed25519.PublicKey) (*API, error) {
	return NewAPIWithVerifier(store, signer, signer, public)
}

func NewAPIWithVerifier(store snapshot.Store, signer snapshot.Signer, verifier snapshot.Verifier, public ed25519.PublicKey) (*API, error) {
	if store == nil || signer == nil || verifier == nil || len(public) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: control API dependencies", core.ErrInvalidConfiguration)
	}
	keyID := snapshot.DeriveKeyID(public)
	if provider, ok := signer.(snapshot.KeyIDProvider); ok && strings.TrimSpace(provider.SigningKeyID()) != "" {
		keyID = strings.TrimSpace(provider.SigningKeyID())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := ensureUsableSnapshot(ctx, store, signer, verifier, keyID); err != nil {
		return nil, err
	}
	return &API{store: store, signer: signer, verifier: verifier, public: append(ed25519.PublicKey(nil), public...), keyID: keyID}, nil
}

func ensureUsableSnapshot(ctx context.Context, store snapshot.Store, signer snapshot.Signer, verifier snapshot.Verifier, activeKeyID string) error {
	const maximumAttempts = 5
	for attempt := 0; attempt < maximumAttempts; attempt++ {
		now := time.Now().UTC()
		current, err := store.Current(ctx)
		missing := errors.Is(err, core.ErrNotFound)
		if err != nil && !missing {
			return fmt.Errorf("load persisted runtime snapshot: %w", err)
		}
		if !missing {
			if err := verifier.Verify(ctx, current); err != nil {
				return fmt.Errorf("verify persisted runtime snapshot: %w", err)
			}
			if err := snapshot.Validate(current, snapshot.ValidationOptions{Now: now}); err != nil {
				return fmt.Errorf("validate persisted runtime snapshot: %w", err)
			}
			if current.ExpiresAt.After(now) && strings.TrimSpace(current.SigningKeyID) == strings.TrimSpace(activeKeyID) {
				return nil
			}
		}

		next := current
		if missing {
			next = snapshot.Snapshot{
				Revision:    1,
				GeneratedAt: now,
				ExpiresAt:   now.Add(24 * time.Hour),
				Models: []snapshot.ModelRoute{{
					VirtualModel: "local/echo",
					Candidates:   []string{"local/echo"},
					PolicyID:     "development-default",
				}},
				ProviderRefs: []snapshot.ProviderReference{{ID: "local", Provider: "local-echo", Region: "local"}},
			}
		} else {
			keyRotated := strings.TrimSpace(next.SigningKeyID) != strings.TrimSpace(activeKeyID)
			next.Revision++
			next.GeneratedAt = now
			if !next.ExpiresAt.After(now) || keyRotated {
				next.ExpiresAt = now.Add(24 * time.Hour)
			}
			next.Signature = nil
			next.SigningKeyID = ""
		}
		next, err = signer.Sign(ctx, next)
		if err != nil {
			return fmt.Errorf("sign runtime snapshot revision %d: %w", next.Revision, err)
		}
		if err := store.Publish(ctx, next); err != nil {
			if errors.Is(err, core.ErrConflict) {
				continue
			}
			return fmt.Errorf("publish runtime snapshot revision %d: %w", next.Revision, err)
		}
		return nil
	}
	return fmt.Errorf("%w: runtime snapshot initialization did not converge after concurrent publication", core.ErrConflict)
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	probe := health.Handler{Ready: func() bool {
		current, err := a.store.Current(context.Background())
		return err == nil && current.ExpiresAt.After(time.Now())
	}}
	mux.HandleFunc("GET /healthz", probe.Health)
	mux.HandleFunc("GET /readyz", probe.Readiness)
	mux.Handle("GET /v1/snapshot", a.requireAction(controlaccess.ActionSnapshotRead, http.HandlerFunc(a.current)))
	mux.Handle("POST /v1/snapshot", a.requireAction(controlaccess.ActionSnapshotPublish, http.HandlerFunc(a.publish)))
	mux.Handle("GET /v1/snapshots/watch", a.requireAction(controlaccess.ActionSnapshotWatch, http.HandlerFunc(a.watch)))
	mux.Handle("GET /v1/signing-key", a.requireAction(controlaccess.ActionSigningKeyRead, http.HandlerFunc(a.signingKey)))
	mux.Handle("GET /v1/models", a.requireAction(controlaccess.ActionModelsRead, http.HandlerFunc(a.models)))
	return mux
}

func (a *API) SetAccessAudit(audit controlaccess.AuditFunc) {
	a.audit = audit
}

func (a *API) requireAction(action controlaccess.Action, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := controlaccess.PrincipalFromContext(r.Context())
		if !ok {
			a.recordAccess(r.Context(), controlaccess.Principal{}, action, controlaccess.Decision{
				Reason: "authentication_required", PolicyVersion: controlaccess.PolicyVersion,
			})
			w.Header().Set("WWW-Authenticate", `Bearer realm="relaydock-control"`)
			apiutil.WriteJSON(w, http.StatusUnauthorized, apiutil.ErrorBody{Error: apiutil.ErrorDetail{
				Code: "authentication_required", Message: "authenticated control-plane principal required",
			}})
			return
		}
		decision := controlaccess.Authorize(principal, action)
		a.recordAccess(r.Context(), principal, action, decision)
		w.Header().Set("X-RelayDock-Policy-Version", decision.PolicyVersion)
		if !decision.Allowed {
			apiutil.WriteJSON(w, http.StatusForbidden, apiutil.ErrorBody{Error: apiutil.ErrorDetail{
				Code: "permission_denied", Message: "control-plane action is not permitted",
			}})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) recordAccess(ctx context.Context, principal controlaccess.Principal, action controlaccess.Action, decision controlaccess.Decision) {
	if a.audit == nil {
		return
	}
	a.audit(ctx, controlaccess.AuditEvent{
		Subject: principal.Subject, Role: principal.Role, TenantID: principal.TenantID, ProjectID: principal.ProjectID,
		Action: action, Allowed: decision.Allowed, Reason: decision.Reason, PolicyVersion: decision.PolicyVersion,
	})
}

func (a *API) current(w http.ResponseWriter, r *http.Request) {
	current, err := a.store.Current(r.Context())
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	etag := fmt.Sprintf(`"snapshot-%d"`, current.Revision)
	w.Header().Set("ETag", etag)
	if strings.TrimSpace(r.Header.Get("If-None-Match")) == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, current)
}

func (a *API) publish(w http.ResponseWriter, r *http.Request) {
	var request PublishRequest
	if err := apiutil.ReadJSON(w, r, apiutil.DefaultMaximumBodyBytes, &request); err != nil {
		apiutil.WriteError(w, err)
		return
	}
	if err := validatePublish(request); err != nil {
		apiutil.WriteError(w, err)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	current, err := a.store.Current(r.Context())
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		apiutil.WriteError(w, err)
		return
	}
	ttl := time.Duration(request.ExpiresInSeconds) * time.Second
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	next := snapshot.Snapshot{
		Revision:       current.Revision + 1,
		GeneratedAt:    time.Now().UTC(),
		ExpiresAt:      time.Now().UTC().Add(ttl),
		VirtualKeys:    request.VirtualKeys,
		Models:         request.Models,
		ProviderRefs:   request.ProviderRefs,
		PriceRevisions: request.PriceRevisions,
	}
	next, err = a.signer.Sign(r.Context(), next)
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	if err := a.store.Publish(r.Context(), next); err != nil {
		apiutil.WriteError(w, err)
		return
	}
	apiutil.WriteJSON(w, http.StatusCreated, next)
}

func (a *API) watch(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		apiutil.WriteError(w, fmt.Errorf("streaming unsupported by response writer"))
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	updates, err := a.store.Watch(r.Context(), after)
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case next, ok := <-updates:
			if !ok {
				return
			}
			payload, _ := json.Marshal(next)
			_, _ = fmt.Fprintf(w, "id: %d\nevent: snapshot\ndata: %s\n\n", next.Revision, payload)
			flusher.Flush()
		}
	}
}

func (a *API) signingKey(w http.ResponseWriter, _ *http.Request) {
	apiutil.WriteJSON(w, http.StatusOK, map[string]string{
		"algorithm": "Ed25519",
		"keyId":     a.keyID,
		"publicKey": base64.RawURLEncoding.EncodeToString(a.public),
	})
}

func (a *API) models(w http.ResponseWriter, r *http.Request) {
	current, err := a.store.Current(r.Context())
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	principal, _ := controlaccess.PrincipalFromContext(r.Context())
	apiutil.WriteJSON(w, http.StatusOK, map[string]any{
		"models": filterModelsForPrincipal(current, principal), "revision": current.Revision,
	})
}

func filterModelsForPrincipal(current snapshot.Snapshot, principal controlaccess.Principal) []snapshot.ModelRoute {
	if principal.IsClusterScoped() {
		return append([]snapshot.ModelRoute(nil), current.Models...)
	}
	allowed := make(map[string]struct{})
	allowAll := false
	for _, key := range current.VirtualKeys {
		if !strings.EqualFold(strings.TrimSpace(key.TenantID), strings.TrimSpace(principal.TenantID)) {
			continue
		}
		if principal.ProjectID != "" && !strings.EqualFold(strings.TrimSpace(key.ProjectID), strings.TrimSpace(principal.ProjectID)) {
			continue
		}
		if len(key.AllowedModels) == 0 {
			allowAll = true
			break
		}
		for _, model := range key.AllowedModels {
			if model = strings.TrimSpace(model); model != "" {
				allowed[strings.ToLower(model)] = struct{}{}
			}
		}
	}
	models := make([]snapshot.ModelRoute, 0, len(current.Models))
	for _, model := range current.Models {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(model.VirtualModel))]; allowAll || ok {
			filtered := model
			filtered.Candidates = append([]string(nil), model.Candidates...)
			filtered.CandidateDetails = append([]snapshot.RouteCandidate(nil), model.CandidateDetails...)
			for index := range filtered.CandidateDetails {
				filtered.CandidateDetails[index].AccountID = ""
			}
			models = append(models, filtered)
		}
	}
	return models
}

func validatePublish(request PublishRequest) error {
	if len(request.Models) == 0 {
		return fmt.Errorf("%w: at least one model route is required", core.ErrInvalidArgument)
	}
	seen := make(map[string]bool, len(request.Models))
	for _, route := range request.Models {
		name := strings.TrimSpace(route.VirtualModel)
		if name == "" || (len(route.Candidates) == 0 && len(route.CandidateDetails) == 0) {
			return fmt.Errorf("%w: every model route requires a name and candidate", core.ErrInvalidArgument)
		}
		if seen[name] {
			return fmt.Errorf("%w: duplicate virtual model %q", core.ErrConflict, name)
		}
		for _, compact := range route.Candidates {
			if strings.TrimSpace(compact) == "" {
				return fmt.Errorf("%w: route %q contains an empty compact candidate", core.ErrInvalidArgument, name)
			}
		}
		for _, candidate := range route.CandidateDetails {
			if strings.TrimSpace(candidate.Provider) == "" || strings.TrimSpace(candidate.Model) == "" {
				return fmt.Errorf("%w: route %q candidate requires provider and model", core.ErrInvalidArgument, name)
			}
			if candidate.EstimatedCost < 0 || candidate.QueueDepth < 0 || candidate.ConcurrencyLimit < 0 || candidate.HealthScore < 0 {
				return fmt.Errorf("%w: route %q candidate contains a negative routing value", core.ErrInvalidArgument, name)
			}
		}
		seen[name] = true
	}
	if request.ExpiresInSeconds < 0 || request.ExpiresInSeconds > int64((30*24*time.Hour)/time.Second) {
		return fmt.Errorf("%w: expiresInSeconds must be between 0 and 2592000", core.ErrInvalidArgument)
	}
	return nil
}

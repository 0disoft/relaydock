package httpgateway

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/auth/virtualkey"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/defaults"
	"github.com/0disoft/relaydock/internal/provider"
	"github.com/0disoft/relaydock/internal/provider/mock"
	"github.com/0disoft/relaydock/internal/routing"
	runtimegateway "github.com/0disoft/relaydock/internal/runtime"
)

func TestRuntimeProcessorPersistsCompleteJournalLifecycle(t *testing.T) {
	adapter := mock.New("journalled response")
	registry := provider.NewRegistry()
	registry.Register(adapter)
	source := runtimegateway.NewMemoryCandidateSource()
	source.Set("code-deep", []routing.Candidate{{
		Provider:     adapter.Name(),
		AccountID:    "mock/default",
		Model:        "mock-model",
		Protocol:     canonical.ProtocolOpenAIResponses,
		Capabilities: adapter.Capabilities("mock-model"),
		HealthScore:  1,
		Available:    true,
	}})
	gateway := runtimegateway.NewGateway()
	gateway.Compiler = defaults.Compiler()
	gateway.Router = routing.New(nil)
	gateway.Providers = registry
	gateway.Candidates = source
	gateway.Leases = routing.NewMemoryLeaseManager()
	fixed := time.Date(2026, 8, 8, 1, 2, 3, 0, time.UTC)
	counter := 0
	gateway.Now = func() time.Time {
		counter++
		return fixed.Add(time.Duration(counter) * time.Millisecond)
	}
	journal := runtimegateway.NewMemoryJournal()
	processor := RuntimeProcessor{Gateway: gateway, Journal: journal, PriceRevisionID: "price-1"}
	ctx := virtualkey.WithPrincipal(context.Background(), virtualkey.Principal{
		TenantID: "tenant", ProjectID: "project", VirtualKeyID: "key",
	})
	completion, err := processor.Complete(ctx, canonical.RequestEnvelope{
		RequestID:       "client-request",
		IngressProtocol: canonical.ProtocolOpenAIResponses,
		Model:           "code-deep",
		Items: []canonical.Item{{
			ID: "item", Kind: canonical.ItemMessage, Role: canonical.RoleUser,
			Content: []canonical.ContentPart{{Type: "text", Text: "solve"}},
		}},
		Requirements: canonical.CapabilityRequirements{Required: []canonical.Capability{canonical.CapabilityTextInput}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if completion.Text != "journalled response" || completion.Attempts != 1 {
		t.Fatalf("completion=%+v", completion)
	}
	requests, attempts := journal.Snapshot()
	if len(requests) != 1 || len(attempts) != 1 {
		t.Fatalf("requests=%+v attempts=%+v", requests, attempts)
	}
	for _, request := range requests {
		if !request.Finished || request.Committed.IsZero() || request.Result.State != runtimegateway.RequestStateCompleted || request.Result.Usage.OutputTokens == 0 {
			t.Fatalf("request=%+v", request)
		}
		if request.Start.ClientRequestID != "client-request" || request.Start.PriceRevisionID != "price-1" {
			t.Fatalf("request start=%+v", request.Start)
		}
	}
	for _, attempt := range attempts {
		if !attempt.Finished || attempt.Result.State != runtimegateway.AttemptStateCompleted || !attempt.Result.Committed {
			t.Fatalf("attempt=%+v", attempt)
		}
	}
}

func TestEffectiveJournalPrincipalMapsOperatorBearerToConfiguredAuditIdentity(t *testing.T) {
	operator := virtualkey.Principal{
		TenantID: "tenant-operator", ProjectID: "00000000-0000-0000-0000-000000000001",
		VirtualKeyID: "00000000-0000-0000-0000-000000000002",
	}
	actual := effectiveJournalPrincipal(virtualkey.Principal{VirtualKeyID: "operator-bearer"}, true, operator)
	if actual.TenantID != operator.TenantID || actual.ProjectID != operator.ProjectID || actual.VirtualKeyID != operator.VirtualKeyID {
		t.Fatalf("operator audit principal not applied: %#v", actual)
	}
}

func TestEffectiveJournalPrincipalDoesNotOverwriteScopedVirtualKey(t *testing.T) {
	authenticated := virtualkey.Principal{
		TenantID: "tenant-user", ProjectID: "project-user", VirtualKeyID: "key-user",
	}
	operator := virtualkey.Principal{TenantID: "tenant-operator", ProjectID: "project-operator", VirtualKeyID: "key-operator"}
	actual := effectiveJournalPrincipal(authenticated, true, operator)
	if actual.TenantID != authenticated.TenantID || actual.ProjectID != authenticated.ProjectID || actual.VirtualKeyID != authenticated.VirtualKeyID {
		t.Fatalf("scoped principal was overwritten: %#v", actual)
	}
}

func TestRuntimeProcessorPrefersSnapshotPriceRevisionSource(t *testing.T) {
	processor := RuntimeProcessor{
		PriceRevisionID:     "static-price",
		PriceRevisionSource: func() string { return "snapshot-price" },
	}
	if actual := processor.currentPriceRevisionID(); actual != "snapshot-price" {
		t.Fatalf("price revision = %q", actual)
	}
	processor.PriceRevisionSource = func() string { return "" }
	if actual := processor.currentPriceRevisionID(); actual != "static-price" {
		t.Fatalf("fallback price revision = %q", actual)
	}
}

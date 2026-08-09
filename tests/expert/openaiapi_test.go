package expert_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	"github.com/your-org/ai-runtime-gateway/internal/expert/routes/openaiapi"
)

func TestOpenAIExpertRequiresPricingForNonZeroCostCeiling(t *testing.T) {
	client := openaiapi.New("http://unused.invalid", "test-key", rejectingDoer{})
	_, err := client.Run(context.Background(), contextpack.Pack{EstimatedTokens: 1000}, openaiapi.RunOptions{
		Model:            "test-model",
		MaximumCostMinor: 3,
	})
	if !errors.Is(err, core.ErrInvalidConfiguration) {
		t.Fatalf("missing pricing returned %v", err)
	}
}

func TestOpenAIExpertRejectsEstimateAboveCeilingBeforeHTTP(t *testing.T) {
	client := openaiapi.New("http://unused.invalid", "test-key", rejectingDoer{}).WithPricing(openaiapi.Pricing{
		InputPerMillionMinor:  1000,
		OutputPerMillionMinor: 2000,
	})
	_, err := client.Run(context.Background(), contextpack.Pack{EstimatedTokens: 1000}, openaiapi.RunOptions{
		Model:               "test-model",
		MaximumCostMinor:    2,
		MaximumOutputTokens: 1000,
	})
	if !errors.Is(err, core.ErrBudgetExceeded) {
		t.Fatalf("over-budget estimate returned %v", err)
	}
}

func TestOpenAIExpertSendsBoundedOutputAndParsesStructuredResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing authorization header")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["max_output_tokens"] != float64(1000) {
			t.Fatalf("unexpected output cap: %#v", payload["max_output_tokens"])
		}
		result := map[string]any{
			"decision":                "Use append-only adjustment entries.",
			"confidence":              0.9,
			"assumptions":             []string{},
			"criticalFindings":        []any{},
			"recommendedArchitecture": map[string]any{"ledger": "append-only"},
			"rejectedAlternatives":    []any{},
			"failureScenarios":        []string{},
			"migrationOrder":          []string{},
			"verificationPlan":        []string{},
			"unresolvedQuestions":     []string{},
			"evidenceReferences":      []string{"ledger.go"},
		}
		encoded, _ := json.Marshal(result)
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": string(encoded)})
	}))
	defer server.Close()

	client := openaiapi.New(server.URL, "test-key", nil).WithPricing(openaiapi.Pricing{
		InputPerMillionMinor:  1000,
		OutputPerMillionMinor: 2000,
	})
	result, err := client.Run(context.Background(), contextpack.Pack{EstimatedTokens: 1000}, openaiapi.RunOptions{
		Model:               "test-model",
		MaximumCostMinor:    3,
		MaximumOutputTokens: 1000,
	})
	if err != nil {
		t.Fatalf("run expert: %v", err)
	}
	if result.Decision != "Use append-only adjustment entries." {
		t.Fatalf("unexpected result: %+v", result)
	}
}

type rejectingDoer struct{}

func (rejectingDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("HTTP request should not be sent")
}

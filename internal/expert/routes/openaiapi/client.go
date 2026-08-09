package openaiapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	"github.com/your-org/ai-runtime-gateway/internal/expert/resultcontract"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
)

const (
	defaultMaximumOutputTokens int64 = 8_192
	maximumResponseBytes       int64 = 16 << 20
)

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type Pricing struct {
	InputPerMillionMinor     int64
	OutputPerMillionMinor    int64
	ReasoningPerMillionMinor int64
}

type Client struct {
	Endpoint string
	APIKey   string
	HTTP     Doer
	Pricing  *Pricing
}

type RunOptions struct {
	Model               string
	ReasoningMode       string
	ReasoningEffort     string
	MaximumCostMinor    int64
	MaximumOutputTokens int64
}

func New(endpoint, apiKey string, httpClient Doer) *Client {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = "https://api.openai.com/v1/responses"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Minute}
	}
	return &Client{Endpoint: endpoint, APIKey: apiKey, HTTP: httpClient}
}

func (c *Client) WithPricing(pricing Pricing) *Client {
	copy := *c
	copy.Pricing = &pricing
	return &copy
}

func (c *Client) Run(ctx context.Context, pack contextpack.Pack, options RunOptions) (resultcontract.Result, error) {
	if c == nil || strings.TrimSpace(c.APIKey) == "" {
		return resultcontract.Result{}, core.ErrProviderNotConfigured
	}
	if strings.TrimSpace(options.Model) == "" {
		return resultcontract.Result{}, fmt.Errorf("%w: model is required", core.ErrInvalidArgument)
	}
	if options.MaximumCostMinor < 0 {
		return resultcontract.Result{}, fmt.Errorf("%w: maximum cost cannot be negative", core.ErrInvalidArgument)
	}
	if options.MaximumOutputTokens <= 0 {
		options.MaximumOutputTokens = defaultMaximumOutputTokens
	}
	if options.MaximumCostMinor > 0 {
		if c.Pricing == nil {
			return resultcontract.Result{}, fmt.Errorf("%w: expert pricing is required to enforce maximum cost", core.ErrInvalidConfiguration)
		}
		estimated, err := c.Pricing.EstimateMaximum(pack.EstimatedTokens, options.MaximumOutputTokens)
		if err != nil {
			return resultcontract.Result{}, err
		}
		if estimated > options.MaximumCostMinor {
			return resultcontract.Result{}, fmt.Errorf("%w: estimated maximum %d exceeds limit %d", core.ErrBudgetExceeded, estimated, options.MaximumCostMinor)
		}
	}

	packJSON, err := json.Marshal(pack)
	if err != nil {
		return resultcontract.Result{}, err
	}
	reasoning := map[string]any{}
	if options.ReasoningMode != "" {
		reasoning["mode"] = options.ReasoningMode
	}
	if options.ReasoningEffort != "" {
		reasoning["effort"] = options.ReasoningEffort
	}
	payload := map[string]any{
		"model":             options.Model,
		"max_output_tokens": options.MaximumOutputTokens,
		"input": []map[string]any{
			{"role": "system", "content": "You are an architecture reviewer. Return only one JSON object matching the requested result contract. Do not wrap it in markdown."},
			{"role": "user", "content": "Review this redacted ContextPack and produce decision, confidence, assumptions, criticalFindings, recommendedArchitecture, rejectedAlternatives, failureScenarios, migrationOrder, verificationPlan, unresolvedQuestions, and evidenceReferences.\n\n" + string(packJSON)},
		},
	}
	if len(reasoning) > 0 {
		payload["reasoning"] = reasoning
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return resultcontract.Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return resultcontract.Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return resultcontract.Result{}, provider.WrapTransportError("openai", err)
	}
	defer resp.Body.Close()
	raw, err := readLimited(resp.Body, maximumResponseBytes)
	if err != nil {
		return resultcontract.Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resultcontract.Result{}, provider.NewHTTPError("openai", resp.StatusCode, truncate(string(raw), 64<<10), resp.Header, time.Now().UTC())
	}
	text, err := extractOutputText(raw)
	if err != nil {
		return resultcontract.Result{}, err
	}
	var result resultcontract.Result
	if err := json.Unmarshal([]byte(stripFence(text)), &result); err != nil {
		return resultcontract.Result{}, fmt.Errorf("decode expert result: %w", err)
	}
	if err := resultcontract.Validate(result); err != nil {
		return resultcontract.Result{}, err
	}
	return result, nil
}

func (p Pricing) EstimateMaximum(inputTokens, maximumOutputTokens int64) (int64, error) {
	if inputTokens < 0 || maximumOutputTokens < 0 || p.InputPerMillionMinor < 0 || p.OutputPerMillionMinor < 0 || p.ReasoningPerMillionMinor < 0 {
		return 0, fmt.Errorf("%w: pricing and token counts cannot be negative", core.ErrInvalidArgument)
	}
	outputRate := p.OutputPerMillionMinor
	if p.ReasoningPerMillionMinor > outputRate {
		outputRate = p.ReasoningPerMillionMinor
	}
	inputCost, err := multiplyAndCeil(inputTokens, p.InputPerMillionMinor, 1_000_000)
	if err != nil {
		return 0, err
	}
	outputCost, err := multiplyAndCeil(maximumOutputTokens, outputRate, 1_000_000)
	if err != nil {
		return 0, err
	}
	if inputCost > int64(^uint64(0)>>1)-outputCost {
		return 0, fmt.Errorf("%w: estimated cost overflow", core.ErrInvalidArgument)
	}
	return inputCost + outputCost, nil
}

func multiplyAndCeil(quantity, rate, denominator int64) (int64, error) {
	if denominator <= 0 {
		return 0, fmt.Errorf("%w: cost denominator must be positive", core.ErrInvalidArgument)
	}
	if quantity == 0 || rate == 0 {
		return 0, nil
	}
	maximum := int64(^uint64(0) >> 1)
	if quantity > maximum/rate {
		return 0, fmt.Errorf("%w: cost calculation overflow", core.ErrInvalidArgument)
	}
	product := quantity * rate
	if product > maximum-(denominator-1) {
		return 0, fmt.Errorf("%w: rounded cost calculation overflow", core.ErrInvalidArgument)
	}
	return (product + denominator - 1) / denominator, nil
}

func readLimited(reader io.Reader, maximum int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximum {
		return nil, core.ErrFrameTooLarge
	}
	return raw, nil
}

func extractOutputText(raw []byte) (string, error) {
	var response struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return "", err
	}
	if response.OutputText != "" {
		return response.OutputText, nil
	}
	var b strings.Builder
	for _, item := range response.Output {
		for _, part := range item.Content {
			if part.Type == "output_text" || part.Type == "text" {
				b.WriteString(part.Text)
			}
		}
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("%w: response contains no output text", core.ErrNotFound)
	}
	return b.String(), nil
}

func stripFence(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```") {
		value = strings.TrimPrefix(value, "```json")
		value = strings.TrimPrefix(value, "```")
		value = strings.TrimSuffix(value, "```")
	}
	return strings.TrimSpace(value)
}

func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}

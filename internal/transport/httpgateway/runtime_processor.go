package httpgateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0disoft/relaydock/internal/auth/virtualkey"
	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/idgen"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/stream"
	"github.com/0disoft/relaydock/internal/provider"
	"github.com/0disoft/relaydock/internal/routing"
	runtimegateway "github.com/0disoft/relaydock/internal/runtime"
)

// RuntimeProcessor connects the compatibility HTTP ingress to the actual
// provider router. Text deltas are losslessly supported. Tool-call and visible
// reasoning deltas are rejected until the canonical event contract carries all
// metadata required for standards-compliant cross-protocol reconstruction.
type RuntimeProcessor struct {
	Gateway             *runtimegateway.Gateway
	MaximumOutputBytes  int64
	Journal             runtimegateway.Journal
	OperatorPrincipal   virtualkey.Principal
	PriceRevisionID     string
	PriceRevisionSource func() string
	JournalTimeout      time.Duration
}

func (p RuntimeProcessor) Complete(ctx context.Context, request canonical.RequestEnvelope) (Completion, error) {
	return p.run(ctx, request, StreamHooks{})
}

func (p RuntimeProcessor) Stream(ctx context.Context, request canonical.RequestEnvelope, hooks StreamHooks) (Completion, error) {
	return p.run(ctx, request, hooks)
}

func (p RuntimeProcessor) run(ctx context.Context, request canonical.RequestEnvelope, hooks StreamHooks) (Completion, error) {
	if p.Gateway == nil {
		return Completion{}, fmt.Errorf("%w: runtime gateway", core.ErrInvalidConfiguration)
	}
	maximum := p.MaximumOutputBytes
	if maximum <= 0 {
		maximum = 16 << 20
	}
	var text bytes.Buffer
	var committed routing.Decision
	createdAt := time.Now().UTC()
	if p.Gateway.Now != nil {
		createdAt = p.Gateway.Now().UTC()
	}
	requestID := firstNonEmpty(request.RequestID, idgen.New("resp"))
	journalRequestID := idgen.New("req")
	journalActive := p.Journal != nil
	if journalActive {
		principal, ok := virtualkey.PrincipalFromContext(ctx)
		principal = effectiveJournalPrincipal(principal, ok, p.OperatorPrincipal)
		if err := p.journal(ctx, false, func(journalCtx context.Context) error {
			return p.Journal.BeginRequest(journalCtx, runtimegateway.JournalRequest{
				ID:              journalRequestID,
				ClientRequestID: request.RequestID,
				TenantID:        principal.TenantID,
				ProjectID:       principal.ProjectID,
				VirtualKeyID:    principal.VirtualKeyID,
				IngressProtocol: string(request.IngressProtocol),
				VirtualModel:    request.Model,
				PriceRevisionID: p.currentPriceRevisionID(),
				StartedAt:       createdAt,
			})
		}); err != nil {
			return Completion{}, fmt.Errorf("begin runtime journal request: %w", err)
		}
	}

	result, err := p.Gateway.RunWithHooks(ctx, request, runtimegateway.RunHooks{
		OnAttemptStarted: func(attemptCtx context.Context, summary runtimegateway.AttemptSummary) error {
			if !journalActive {
				return nil
			}
			return p.journal(attemptCtx, true, func(journalCtx context.Context) error {
				return p.Journal.BeginAttempt(journalCtx, runtimegateway.JournalAttempt{
					ID:            summary.ID,
					RequestID:     journalRequestID,
					Number:        summary.Number,
					Provider:      summary.Decision.Candidate.Provider,
					AccountID:     summary.Decision.Candidate.AccountID,
					UpstreamModel: summary.Decision.Candidate.Model,
					Protocol:      string(summary.Decision.Candidate.Protocol),
					StartedAt:     summary.StartedAt,
				})
			})
		},
		OnAttemptCompleted: func(attemptCtx context.Context, summary runtimegateway.AttemptSummary, attemptErr error) error {
			if !journalActive {
				return nil
			}
			state := runtimegateway.AttemptStateCompleted
			if attemptErr != nil {
				state = runtimegateway.AttemptStateFailed
				if errors.Is(attemptErr, context.Canceled) {
					state = runtimegateway.AttemptStateCancelled
				}
			}
			return p.journal(attemptCtx, true, func(journalCtx context.Context) error {
				return p.Journal.FinishAttempt(journalCtx, runtimegateway.JournalAttemptResult{
					ID:          summary.ID,
					RequestID:   journalRequestID,
					State:       state,
					Committed:   summary.Committed,
					Usage:       summary.Usage,
					ErrorCode:   summary.ErrorCode,
					Error:       boundedJournalError(summary.Error),
					CompletedAt: summary.CompletedAt,
				})
			})
		},
		OnCommit: func(commitCtx context.Context, decision routing.Decision) error {
			if journalActive {
				if err := p.journal(commitCtx, true, func(journalCtx context.Context) error {
					return p.Journal.MarkCommitted(journalCtx, journalRequestID, p.now())
				}); err != nil {
					return err
				}
			}
			committed = decision
			if hooks.OnCommit == nil {
				return nil
			}
			model := request.Model
			if model == "" {
				model = decision.Candidate.Model
			}
			return hooks.OnCommit(commitCtx, StreamMetadata{
				RequestID: requestID, Model: model, Provider: decision.Candidate.Provider,
				UpstreamModel: decision.Candidate.Model, CreatedAt: createdAt,
			})
		},
		OnEvent: func(eventCtx context.Context, event stream.Event) error {
			switch event.Kind {
			case stream.EventContentDelta:
				if int64(text.Len()+len(event.Delta)) > maximum {
					return core.ErrFrameTooLarge
				}
				_, _ = text.Write(event.Delta)
			case stream.EventReasoningDelta:
				if hooks.OnEvent == nil {
					return fmt.Errorf("%w: compatibility egress cannot represent %s", core.ErrLossyTransformation, event.Kind)
				}
				// The live writer validates the destination and original field.
			case stream.EventToolCallDelta:
				return fmt.Errorf("%w: compatibility egress cannot represent %s", core.ErrLossyTransformation, event.Kind)
			case stream.EventResponseStarted, stream.EventUsage, stream.EventProviderRaw, stream.EventCompleted:
				// Forwarded below when a live stream hook is present.
			case stream.EventFailed:
				return fmt.Errorf("provider stream failed: %s", event.Delta)
			default:
				return fmt.Errorf("%w: stream event %s", core.ErrInvalidArgument, event.Kind)
			}
			if hooks.OnEvent != nil {
				return hooks.OnEvent(eventCtx, event)
			}
			return nil
		},
	})
	if journalActive {
		state := runtimegateway.RequestStateCompleted
		errorCode := ""
		errorMessage := ""
		if err != nil {
			state = runtimegateway.RequestStateFailed
			if errors.Is(err, context.Canceled) {
				state = runtimegateway.RequestStateCancelled
			}
			errorCode = provider.ErrorCode(err)
			errorMessage = boundedJournalError(err.Error())
		}
		finishErr := p.journal(ctx, true, func(journalCtx context.Context) error {
			return p.Journal.FinishRequest(journalCtx, runtimegateway.JournalRequestResult{
				ID:          journalRequestID,
				State:       state,
				Usage:       result.TotalUsage,
				Attempts:    result.Attempts,
				ErrorCode:   errorCode,
				Error:       errorMessage,
				CompletedAt: p.now(),
			})
		})
		if finishErr != nil {
			err = errors.Join(err, fmt.Errorf("finish runtime journal request: %w", finishErr))
		}
	}
	if err != nil {
		return Completion{}, err
	}
	decision := result.Decision
	if decision.Candidate.Provider == "" {
		decision = committed
	}
	model := request.Model
	if model == "" {
		model = decision.Candidate.Model
	}
	return Completion{
		ID:            requestID,
		Model:         model,
		UpstreamModel: decision.Candidate.Model,
		Provider:      decision.Candidate.Provider,
		Text:          text.String(),
		FinishReason:  "stop",
		InputTokens:   result.Usage.InputTokens,
		OutputTokens:  result.Usage.OutputTokens,
		CreatedAt:     createdAt,
		Attempts:      result.Attempts,
	}, nil
}

func effectiveJournalPrincipal(authenticated virtualkey.Principal, present bool, operator virtualkey.Principal) virtualkey.Principal {
	if !present {
		return operator
	}
	if authenticated.TenantID == "" {
		authenticated.TenantID = operator.TenantID
	}
	if authenticated.ProjectID == "" {
		authenticated.ProjectID = operator.ProjectID
	}
	// Static operator bearer middleware deliberately uses a non-database key
	// marker. Replace it with the configured audit identity before a durable
	// journal validates UUID ownership. Scoped virtual keys retain their own ID.
	if authenticated.VirtualKeyID == "" || authenticated.VirtualKeyID == "operator-bearer" {
		authenticated.VirtualKeyID = operator.VirtualKeyID
	}
	return authenticated
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (p RuntimeProcessor) journal(ctx context.Context, durable bool, call func(context.Context) error) error {
	if call == nil {
		return nil
	}
	journalCtx := ctx
	if durable {
		journalCtx = context.WithoutCancel(ctx)
	}
	timeout := p.JournalTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	journalCtx, cancel := context.WithTimeout(journalCtx, timeout)
	defer cancel()
	return call(journalCtx)
}

func (p RuntimeProcessor) currentPriceRevisionID() string {
	if p.PriceRevisionSource != nil {
		if value := p.PriceRevisionSource(); value != "" {
			return value
		}
	}
	return firstNonEmpty(p.PriceRevisionID, "unpriced")
}

func (p RuntimeProcessor) now() time.Time {
	if p.Gateway != nil && p.Gateway.Now != nil {
		return p.Gateway.Now().UTC()
	}
	return time.Now().UTC()
}

func boundedJournalError(value string) string {
	const maximum = 4096
	if len(value) <= maximum {
		return value
	}
	return value[:maximum] + "…"
}

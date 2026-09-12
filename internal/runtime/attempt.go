package runtime

import (
	"context"
	"fmt"
	"io"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/stream"
	"github.com/0disoft/relaydock/internal/provider"
	"github.com/0disoft/relaydock/internal/routing"
)

func (g *Gateway) executeAttempt(
	ctx context.Context,
	envelope canonical.RequestEnvelope,
	decision routing.Decision,
	lease routing.Lease,
	hooks RunHooks,
) (usage stream.Usage, committed bool, returnedErr error) {
	if lease.ID != "" {
		defer func() {
			if err := g.releaseLease(ctx, lease); err != nil && returnedErr == nil && !committed {
				returnedErr = err
			}
		}()
	}

	adapter, ok := g.Providers.Get(decision.Candidate.Provider)
	if !ok {
		return stream.Usage{}, false, fmt.Errorf("%w: provider %s", core.ErrNotFound, decision.Candidate.Provider)
	}
	targetProtocol := decision.Candidate.Protocol
	if targetProtocol == "" {
		targetProtocol = envelope.IngressProtocol
	}
	// Compile the selected upstream identity, not the public/virtual route.
	// Keep the original envelope intact for accounting and subsequent attempts.
	upstreamEnvelope := envelope
	upstreamEnvelope.Model = decision.Candidate.Model
	encoded, _, err := g.Compiler.Encode(ctx, upstreamEnvelope, targetProtocol, g.LossMode)
	if err != nil {
		return stream.Usage{}, false, err
	}

	attemptCtx, cancelAttempt := context.WithCancel(ctx)
	defer cancelAttempt()
	leaseErrors := make(chan error, 1)
	stopRenewal := g.startLeaseRenewal(attemptCtx, cancelAttempt, lease, leaseErrors)
	defer stopRenewal()

	attemptStream, err := adapter.Execute(attemptCtx, provider.AttemptRequest{
		ProviderAccountID: decision.Candidate.AccountID,
		Model:             decision.Candidate.Model,
		Request:           encoded,
	})
	if err != nil {
		return stream.Usage{}, false, err
	}
	defer attemptStream.Close()

	machine := stream.NewStateMachine()
	buffered := make([]stream.Event, 0, 4)
	commit := func() error {
		if committed {
			return nil
		}
		// The retry boundary is crossed as soon as a semantic event is ready.
		// A downstream header write or durable journal failure after this point
		// must not cause the same logical request to run on another provider.
		committed = true
		if err := hooks.OnCommit(ctx, decision); err != nil {
			return err
		}
		for _, bufferedEvent := range buffered {
			if err := hooks.OnEvent(ctx, bufferedEvent); err != nil {
				return err
			}
		}
		buffered = buffered[:0]
		return nil
	}
	deliver := func(event stream.Event) error {
		if committed {
			return hooks.OnEvent(ctx, event)
		}
		buffered = append(buffered, cloneEvent(event))
		if isSemanticEvent(event.Kind) || event.Kind == stream.EventCompleted {
			return commit()
		}
		return nil
	}

	for {
		event, readErr := attemptStream.Next(attemptCtx)
		if readErr != nil {
			if leaseErr := receiveLeaseError(leaseErrors); leaseErr != nil {
				return usage, committed, leaseErr
			}
			if readErr == io.EOF && machine.State() == stream.StateCompleted {
				if !committed {
					if err := commit(); err != nil {
						return usage, committed, err
					}
				}
				return usage, committed, nil
			}
			if readErr == io.EOF {
				readErr = io.ErrUnexpectedEOF
			}
			return usage, committed, provider.WrapTransportError(decision.Candidate.Provider, readErr)
		}
		if err := machine.Accept(event); err != nil {
			return usage, committed, err
		}
		if event.Usage != nil {
			usage.Add(*event.Usage)
		}

		switch event.Kind {
		case stream.EventFailed:
			failure := &provider.UpstreamError{
				Provider: decision.Candidate.Provider,
				Class:    provider.ErrorClassUpstream,
				Message:  string(event.Delta),
			}
			if committed {
				if err := hooks.OnEvent(ctx, event); err != nil {
					return usage, committed, err
				}
			}
			return usage, committed, failure
		default:
			if err := deliver(event); err != nil {
				return usage, committed, err
			}
		}
		if event.Kind == stream.EventCompleted {
			return usage, committed, nil
		}
	}
}

package httpgateway

import (
	"context"
	"net/http"
	"strconv"

	"github.com/0disoft/relaydock/internal/auth/authorization"
	"github.com/0disoft/relaydock/internal/auth/virtualkey"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/stream"
	"github.com/0disoft/relaydock/internal/transport/apiutil"
)

func (a *api) process(w http.ResponseWriter, r *http.Request, protocol canonical.Protocol, model string, forceStream bool) {
	body, err := readBody(w, r, a.options.MaximumBodyBytes)
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	envelope, err := a.options.Compiler.Decode(r.Context(), protocol, body)
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	if model != "" {
		envelope.Model = model
	}
	if forceStream {
		envelope.Stream = true
	}
	if principal, ok := virtualkey.PrincipalFromContext(r.Context()); ok {
		if err := principal.Authorize(authorization.ScopeModelsInvoke, envelope.Model); err != nil {
			apiutil.WriteError(w, err)
			return
		}
	}
	w.Header().Set("X-AI-Runtime-Request-ID", envelope.RequestID)
	if envelope.Stream {
		if streaming, ok := a.options.Processor.(StreamingProcessor); ok {
			a.processLiveStream(w, r, protocol, envelope, streaming)
			return
		}
	}
	completion, err := a.options.Processor.Complete(r.Context(), envelope)
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	providerName := completion.Provider
	if providerName == "" {
		providerName = "unknown"
	}
	w.Header().Set("X-AI-Runtime-Provider", providerName)
	if completion.UpstreamModel != "" {
		w.Header().Set("X-AI-Runtime-Upstream-Model", completion.UpstreamModel)
	}
	if completion.Attempts > 0 {
		w.Header().Set("X-AI-Runtime-Attempts", strconv.Itoa(completion.Attempts))
	}
	if envelope.Stream {
		writeStream(w, protocol, completion)
		return
	}
	writeCompletion(w, protocol, completion)
}

func (a *api) processLiveStream(
	w http.ResponseWriter,
	r *http.Request,
	protocol canonical.Protocol,
	envelope canonical.RequestEnvelope,
	processor StreamingProcessor,
) {
	writer, err := newLiveStreamWriter(w, protocol, envelope.RequestID, envelope.Model)
	if err != nil {
		apiutil.WriteError(w, err)
		return
	}
	completion, runErr := processor.Stream(r.Context(), envelope, StreamHooks{
		OnCommit: func(_ context.Context, metadata StreamMetadata) error {
			return writer.Commit(metadata)
		},
		OnEvent: func(_ context.Context, event stream.Event) error {
			return writer.Event(event)
		},
	})
	writer.SetAttempts(completion.Attempts)
	if runErr == nil {
		if !writer.committed {
			// A conforming runtime commits no later than its terminal event. Keep a
			// defensive path for custom StreamingProcessor implementations.
			if err := writer.Commit(StreamMetadata{
				RequestID: completion.ID, Model: completion.Model, Provider: completion.Provider,
				UpstreamModel: completion.UpstreamModel, CreatedAt: completion.CreatedAt,
			}); err != nil {
				apiutil.WriteError(w, err)
				return
			}
		}
		if !writer.terminal {
			if err := writer.complete(); err != nil {
				_ = writer.Fail(err)
			}
		}
		return
	}
	if !writer.committed {
		apiutil.WriteError(w, runErr)
		return
	}
	_ = writer.Fail(runErr)
}

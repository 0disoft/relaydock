package compiler

import (
	"context"
	"fmt"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/idgen"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
)

type EncodedRequest struct {
	Protocol canonical.Protocol
	Body     []byte
	Headers  map[string]string
}
type Compiler interface {
	Decode(context.Context, canonical.Protocol, []byte) (canonical.RequestEnvelope, error)
	Analyze(context.Context, canonical.RequestEnvelope, canonical.Protocol, LossMode) (Report, error)
	Encode(context.Context, canonical.RequestEnvelope, canonical.Protocol, LossMode) (EncodedRequest, Report, error)
}
type Service struct{ registry *Registry }

func New(registry *Registry) *Service {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Service{registry: registry}
}

func (s *Service) Decode(ctx context.Context, p canonical.Protocol, body []byte) (canonical.RequestEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return canonical.RequestEnvelope{}, err
	}
	if len(body) == 0 {
		return canonical.RequestEnvelope{}, fmt.Errorf("%w: empty request body", core.ErrInvalidArgument)
	}
	d, err := s.registry.Decoder(p)
	if err != nil {
		return canonical.RequestEnvelope{}, err
	}
	envelope, err := d.Decode(body)
	if err != nil {
		return canonical.RequestEnvelope{}, fmt.Errorf("decode %s: %w", p, err)
	}
	if envelope.RequestID == "" {
		envelope.RequestID = idgen.New("req")
	}
	envelope.IngressProtocol = p
	if err := envelope.Validate(); err != nil {
		return canonical.RequestEnvelope{}, err
	}
	return envelope, nil
}
func (s *Service) Analyze(ctx context.Context, e canonical.RequestEnvelope, dst canonical.Protocol, mode LossMode) (Report, error) {
	_, r, err := s.Encode(ctx, e, dst, mode)
	return r, err
}
func (s *Service) Encode(ctx context.Context, e canonical.RequestEnvelope, dst canonical.Protocol, mode LossMode) (EncodedRequest, Report, error) {
	if err := ctx.Err(); err != nil {
		return EncodedRequest{}, Report{}, err
	}
	if err := e.Validate(); err != nil {
		return EncodedRequest{}, Report{}, err
	}
	if mode == "" {
		mode = LossModeStrict
	}
	encoder, err := s.registry.Encoder(dst)
	if err != nil {
		return EncodedRequest{}, Report{}, err
	}
	body, report, err := encoder.Encode(e, mode)
	if err != nil {
		return EncodedRequest{}, report, fmt.Errorf("encode %s: %w", dst, err)
	}
	if report.HasFatalLoss() {
		return EncodedRequest{}, report, core.ErrLossyTransformation
	}
	return EncodedRequest{Protocol: dst, Body: body, Headers: map[string]string{"Content-Type": "application/json"}}, report, nil
}

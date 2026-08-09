package mcpbridge

import (
	"context"

	"github.com/0disoft/relaydock/internal/localipc"
)

type IPCBackend struct {
	client localipc.Client
}

func NewIPCBackend(client localipc.Client) *IPCBackend {
	return &IPCBackend{client: client}
}

func (b *IPCBackend) CreateConsultation(ctx context.Context, input ConsultationCreateInput) (ConsultationCreateOutput, error) {
	var output ConsultationCreateOutput
	err := b.client.Call(ctx, "consultation.create", input, &output)
	return output, err
}

func (b *IPCBackend) GetConsultation(ctx context.Context, input ConsultationGetInput) (ConsultationGetOutput, error) {
	var output ConsultationGetOutput
	err := b.client.Call(ctx, "consultation.get", input, &output)
	return output, err
}

func (b *IPCBackend) CancelConsultation(ctx context.Context, input ConsultationCancelInput) (ConsultationCancelOutput, error) {
	var output ConsultationCancelOutput
	err := b.client.Call(ctx, "consultation.cancel", input, &output)
	return output, err
}

func (b *IPCBackend) PreviewContext(ctx context.Context, input ContextPreviewInput) (ContextPreviewOutput, error) {
	var output ContextPreviewOutput
	err := b.client.Call(ctx, "context.preview", input, &output)
	return output, err
}

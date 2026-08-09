package mcpbridge

import (
	"context"

	"github.com/0disoft/relaydock/internal/mcpcontract"
)

type ConsultationCreateInput = mcpcontract.ConsultationCreateInput
type ConsultationCreateOutput = mcpcontract.ConsultationCreateOutput
type ConsultationGetInput = mcpcontract.ConsultationGetInput
type ConsultationGetOutput = mcpcontract.ConsultationGetOutput
type ConsultationCancelInput = mcpcontract.ConsultationCancelInput
type ConsultationCancelOutput = mcpcontract.ConsultationCancelOutput
type ContextPreviewInput = mcpcontract.ContextPreviewInput
type ContextPreviewOutput = mcpcontract.ContextPreviewOutput

type ToolBackend interface {
	CreateConsultation(context.Context, ConsultationCreateInput) (ConsultationCreateOutput, error)
	GetConsultation(context.Context, ConsultationGetInput) (ConsultationGetOutput, error)
	CancelConsultation(context.Context, ConsultationCancelInput) (ConsultationCancelOutput, error)
	PreviewContext(context.Context, ContextPreviewInput) (ContextPreviewOutput, error)
}

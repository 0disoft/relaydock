package mcpbridge

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	backend ToolBackend
	raw     *mcp.Server
}

func NewServer(backend ToolBackend, version string) *Server {
	raw := mcp.NewServer(&mcp.Implementation{
		Name:    "ai-runtime-expert",
		Version: version,
	}, nil)

	server := &Server{backend: backend, raw: raw}
	server.registerTools()
	return server
}

func (s *Server) registerTools() {
	mcp.AddTool(s.raw, &mcp.Tool{
		Name:        "expert_consultation_create",
		Description: "Create an expert architecture or debugging consultation from the active repository context.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ConsultationCreateInput) (*mcp.CallToolResult, ConsultationCreateOutput, error) {
		output, err := s.backend.CreateConsultation(ctx, input)
		return nil, output, err
	})

	mcp.AddTool(s.raw, &mcp.Tool{
		Name:        "expert_consultation_get",
		Description: "Read the current state and structured result of a consultation.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ConsultationGetInput) (*mcp.CallToolResult, ConsultationGetOutput, error) {
		output, err := s.backend.GetConsultation(ctx, input)
		return nil, output, err
	})

	mcp.AddTool(s.raw, &mcp.Tool{
		Name:        "expert_consultation_cancel",
		Description: "Cancel a consultation that is no longer needed. Completed consultations are immutable.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ConsultationCancelInput) (*mcp.CallToolResult, ConsultationCancelOutput, error) {
		output, err := s.backend.CancelConsultation(ctx, input)
		return nil, output, err
	})

	mcp.AddTool(s.raw, &mcp.Tool{
		Name:        "context_pack_preview",
		Description: "Preview files and secret findings before any repository context is uploaded.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ContextPreviewInput) (*mcp.CallToolResult, ContextPreviewOutput, error) {
		output, err := s.backend.PreviewContext(ctx, input)
		return nil, output, err
	})
}

func (s *Server) RunStdio(ctx context.Context) error {
	return s.raw.Run(ctx, &mcp.StdioTransport{})
}

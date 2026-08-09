package defaults

import (
	anthropic "github.com/your-org/ai-runtime-gateway/internal/protocol/anthropic/messages"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/compiler"
	gemini "github.com/your-org/ai-runtime-gateway/internal/protocol/gemini/generate"
	chat "github.com/your-org/ai-runtime-gateway/internal/protocol/openai/chat"
	responses "github.com/your-org/ai-runtime-gateway/internal/protocol/openai/responses"
)

func Registry() *compiler.Registry {
	r := compiler.NewRegistry()
	adapters := []interface{}{chat.New(), responses.New(), anthropic.New(), gemini.New()}
	for _, raw := range adapters {
		if d, ok := raw.(compiler.Decoder); ok {
			r.RegisterDecoder(d)
		}
		if e, ok := raw.(compiler.Encoder); ok {
			r.RegisterEncoder(e)
		}
	}
	return r
}
func Compiler() *compiler.Service { return compiler.New(Registry()) }

package compiler

import (
	"fmt"
	"sync"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/canonical"
)

type Decoder interface {
	Protocol() canonical.Protocol
	Decode([]byte) (canonical.RequestEnvelope, error)
}
type Encoder interface {
	Protocol() canonical.Protocol
	Encode(canonical.RequestEnvelope, LossMode) ([]byte, Report, error)
}

type Registry struct {
	mu       sync.RWMutex
	decoders map[canonical.Protocol]Decoder
	encoders map[canonical.Protocol]Encoder
}

func NewRegistry() *Registry {
	return &Registry{decoders: map[canonical.Protocol]Decoder{}, encoders: map[canonical.Protocol]Encoder{}}
}
func (r *Registry) RegisterDecoder(d Decoder) {
	if d == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.decoders[d.Protocol()] = d
}
func (r *Registry) RegisterEncoder(e Encoder) {
	if e == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.encoders[e.Protocol()] = e
}
func (r *Registry) Decoder(p canonical.Protocol) (Decoder, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.decoders[p]
	if !ok {
		return nil, fmt.Errorf("%w: decoder for %s", core.ErrNotFound, p)
	}
	return d, nil
}
func (r *Registry) Encoder(p canonical.Protocol) (Encoder, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.encoders[p]
	if !ok {
		return nil, fmt.Errorf("%w: encoder for %s", core.ErrNotFound, p)
	}
	return e, nil
}

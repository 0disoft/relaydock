package modelregistry

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/0disoft/relaydock/internal/core"
)

type MemoryRegistry struct {
	mu       sync.RWMutex
	models   map[string]Model
	virtuals map[string][]string
}

func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{models: make(map[string]Model), virtuals: make(map[string][]string)}
}

func (r *MemoryRegistry) Put(model Model) error {
	if strings.TrimSpace(model.Provider) == "" || strings.TrimSpace(model.UpstreamID) == "" {
		return fmt.Errorf("%w: provider and upstream model are required", core.ErrInvalidArgument)
	}
	r.mu.Lock()
	r.models[key(model.Provider, model.UpstreamID)] = cloneModel(model)
	r.mu.Unlock()
	return nil
}

func (r *MemoryRegistry) SetVirtualModel(name string, candidates []Model) error {
	name = strings.TrimSpace(name)
	if name == "" || len(candidates) == 0 {
		return fmt.Errorf("%w: virtual model and candidates are required", core.ErrInvalidArgument)
	}
	keys := make([]string, 0, len(candidates))
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, candidate := range candidates {
		if candidate.Provider == "" || candidate.UpstreamID == "" {
			return fmt.Errorf("%w: candidate provider and model are required", core.ErrInvalidArgument)
		}
		candidateKey := key(candidate.Provider, candidate.UpstreamID)
		r.models[candidateKey] = cloneModel(candidate)
		keys = append(keys, candidateKey)
	}
	r.virtuals[name] = keys
	return nil
}

func (r *MemoryRegistry) Get(provider, upstreamID string) (Model, bool) {
	r.mu.RLock()
	model, ok := r.models[key(provider, upstreamID)]
	r.mu.RUnlock()
	return cloneModel(model), ok
}

func (r *MemoryRegistry) ResolveVirtualModel(name string) ([]Model, error) {
	r.mu.RLock()
	candidateKeys := append([]string(nil), r.virtuals[name]...)
	out := make([]Model, 0, len(candidateKeys))
	for _, candidateKey := range candidateKeys {
		if model, ok := r.models[candidateKey]; ok {
			out = append(out, cloneModel(model))
		}
	}
	r.mu.RUnlock()
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: virtual model %q", core.ErrNotFound, name)
	}
	return out, nil
}

func (r *MemoryRegistry) List() []Model {
	r.mu.RLock()
	out := make([]Model, 0, len(r.models))
	for _, model := range r.models {
		out = append(out, cloneModel(model))
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].UpstreamID < out[j].UpstreamID
	})
	return out
}

func key(provider, model string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "\x00" + strings.TrimSpace(model)
}

func cloneModel(model Model) Model {
	model.Regions = append([]string(nil), model.Regions...)
	model.KnownIncompatibilities = append([]string(nil), model.KnownIncompatibilities...)
	model.Capabilities = model.Capabilities.Clone()
	return model
}

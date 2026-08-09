package modelregistry

import "github.com/0disoft/relaydock/internal/protocol/canonical"

type Model struct {
	Provider               string
	UpstreamID             string
	Snapshot               string
	Capabilities           canonical.CapabilitySet
	ContextLimit           int64
	OutputLimit            int64
	Regions                []string
	KnownIncompatibilities []string
}

type Registry interface {
	Get(provider, upstreamID string) (Model, bool)
	ResolveVirtualModel(string) ([]Model, error)
}

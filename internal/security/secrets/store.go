package secrets

import "context"

type Reference struct {
	ID       string
	TenantID string
	Kind     string
}

type Store interface {
	Put(context.Context, Reference, []byte) error
	Resolve(context.Context, Reference) ([]byte, error)
	Delete(context.Context, Reference) error
}

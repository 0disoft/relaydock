package credentials

import "context"

type Reference struct {
	ID       string
	Provider string
	Account  string
}

type Store interface {
	Put(context.Context, Reference, []byte) error
	Get(context.Context, Reference) ([]byte, error)
	Delete(context.Context, Reference) error
}

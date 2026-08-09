package contextpack

import "context"

type Store interface {
	Put(context.Context, Pack) error
	Get(context.Context, string) (Pack, error)
	Delete(context.Context, string) error
}

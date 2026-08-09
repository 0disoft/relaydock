package snapshot

import "context"

type Store interface {
	Current(context.Context) (Snapshot, error)
	Publish(context.Context, Snapshot) error
	Watch(context.Context, int64) (<-chan Snapshot, error)
}

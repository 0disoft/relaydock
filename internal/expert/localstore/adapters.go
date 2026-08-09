package localstore

import (
	"context"

	"github.com/0disoft/relaydock/internal/expert/contextpack"
	"github.com/0disoft/relaydock/internal/expert/resultcontract"
)

// ContextPackStore and ResultStore avoid method-name collisions while all
// aggregates continue to share one transactional metadata file.
type ContextPackStore struct{ store *Store }
type ResultStore struct{ store *Store }

func (s *Store) ContextPacks() contextpack.Store { return ContextPackStore{store: s} }
func (s *Store) Results() resultcontract.Store   { return ResultStore{store: s} }

func (a ContextPackStore) Put(ctx context.Context, pack contextpack.Pack) error {
	return a.store.Put(ctx, pack)
}
func (a ContextPackStore) Get(ctx context.Context, id string) (contextpack.Pack, error) {
	return a.store.GetPack(ctx, id)
}
func (a ContextPackStore) Delete(ctx context.Context, id string) error {
	return a.store.Delete(ctx, id)
}
func (a ResultStore) Put(ctx context.Context, result resultcontract.Result) (string, error) {
	return a.store.PutResult(ctx, result)
}
func (a ResultStore) Get(ctx context.Context, id string) (resultcontract.Result, error) {
	return a.store.GetResult(ctx, id)
}
func (a ResultStore) PutRecord(ctx context.Context, record resultcontract.Record) (string, error) {
	return a.store.PutResultRecord(ctx, record)
}
func (a ResultStore) GetRecord(ctx context.Context, id string) (resultcontract.Record, error) {
	return a.store.GetResultRecord(ctx, id)
}

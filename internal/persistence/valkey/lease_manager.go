package valkey

import (
	"context"
	"fmt"

	"github.com/0disoft/relaydock/internal/routing/distributedlease"
	valkeygo "github.com/valkey-io/valkey-go"
)

type scriptExecutor struct {
	client valkeygo.Client
}

func (e scriptExecutor) ExecuteInt64(ctx context.Context, source string, keys, args []string) (int64, error) {
	if e.client == nil {
		return 0, fmt.Errorf("valkey script client is not initialized")
	}
	value, err := valkeygo.NewLuaScript(source).Exec(ctx, e.client, keys, args).ToInt64()
	if err != nil {
		return 0, err
	}
	return value, nil
}

func NewLeaseManager(client *Client, prefix string) (*distributedlease.Manager, error) {
	if client == nil || client.Raw == nil {
		return nil, fmt.Errorf("valkey client is not initialized")
	}
	return distributedlease.New(scriptExecutor{client: client.Raw}, prefix)
}

package valkey

import (
	"context"
	"fmt"
	"strings"

	valkeygo "github.com/valkey-io/valkey-go"
)

type Client struct {
	Raw valkeygo.Client
}

// Open accepts either a Valkey URL or a comma-separated list of host:port
// addresses. Authentication and TLS should be expressed in URL form.
func Open(address string) (*Client, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("valkey address is required")
	}
	var option valkeygo.ClientOption
	var err error
	if strings.Contains(address, "://") {
		option, err = valkeygo.ParseURL(address)
		if err != nil {
			return nil, fmt.Errorf("parse valkey URL: %w", err)
		}
	} else {
		parts := strings.Split(address, ",")
		addresses := make([]string, 0, len(parts))
		for _, part := range parts {
			if value := strings.TrimSpace(part); value != "" {
				addresses = append(addresses, value)
			}
		}
		if len(addresses) == 0 {
			return nil, fmt.Errorf("valkey address is required")
		}
		option = valkeygo.ClientOption{InitAddress: addresses, ShuffleInit: len(addresses) > 1}
	}
	raw, err := valkeygo.NewClient(option)
	if err != nil {
		return nil, fmt.Errorf("open valkey client: %w", err)
	}
	return &Client{Raw: raw}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.Raw == nil {
		return fmt.Errorf("valkey client is not initialized")
	}
	if _, err := c.Raw.Do(ctx, c.Raw.B().Ping().Build()).ToString(); err != nil {
		return fmt.Errorf("ping valkey: %w", err)
	}
	return nil
}

func (c *Client) Close() {
	if c != nil && c.Raw != nil {
		c.Raw.Close()
	}
}

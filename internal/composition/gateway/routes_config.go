package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
)

func loadRouteConfig(path, inline string) (map[string][]RouteConfig, error) {
	merged := make(map[string][]RouteConfig)
	if strings.TrimSpace(path) != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read gateway routes: %w", err)
		}
		if len(raw) > 1<<20 {
			return nil, fmt.Errorf("%w: gateway routes file exceeds 1 MiB", core.ErrFrameTooLarge)
		}
		parsed, err := decodeRouteConfig(raw)
		if err != nil {
			return nil, fmt.Errorf("decode gateway routes file: %w", err)
		}
		for model, routes := range parsed {
			merged[model] = routes
		}
	}
	if strings.TrimSpace(inline) != "" {
		if len(inline) > 1<<20 {
			return nil, fmt.Errorf("%w: inline gateway routes exceed 1 MiB", core.ErrFrameTooLarge)
		}
		parsed, err := decodeRouteConfig([]byte(inline))
		if err != nil {
			return nil, fmt.Errorf("decode inline gateway routes: %w", err)
		}
		for model, routes := range parsed {
			merged[model] = routes
		}
	}
	return merged, nil
}

func decodeRouteConfig(raw []byte) (map[string][]RouteConfig, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value map[string][]RouteConfig
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, fmt.Errorf("decode trailing JSON: %w", err)
	}
	return value, nil
}

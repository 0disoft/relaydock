package httpadapter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/stream"
	"github.com/0disoft/relaydock/internal/provider"
)

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}
type PathBuilder func(provider.AttemptRequest) (string, error)
type Config struct {
	Name, BaseURL, APIKey, AuthHeader, AuthPrefix string
	Headers                                       map[string]string
	Paths                                         map[canonical.Protocol]string
	PathBuilder                                   PathBuilder
	Capabilities                                  canonical.CapabilitySet
	Client                                        Doer
	MaximumResponseBytes                          int64
	AllowAnonymous                                bool
}
type Adapter struct{ cfg Config }

func New(cfg Config) *Adapter {
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 0, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConns: 200, MaxIdleConnsPerHost: 50, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 5 * time.Minute}}
	}
	if cfg.AuthHeader == "" {
		cfg.AuthHeader = "Authorization"
	}
	if cfg.AuthPrefix == "" && cfg.AuthHeader == "Authorization" {
		cfg.AuthPrefix = "Bearer "
	}
	if cfg.MaximumResponseBytes <= 0 {
		cfg.MaximumResponseBytes = 16 << 20
	}
	return &Adapter{cfg: cfg}
}
func (a *Adapter) Name() string                                { return a.cfg.Name }
func (a *Adapter) Capabilities(string) canonical.CapabilitySet { return a.cfg.Capabilities.Clone() }
func (a *Adapter) Execute(ctx context.Context, attempt provider.AttemptRequest) (provider.AttemptStream, error) {
	if strings.TrimSpace(a.cfg.BaseURL) == "" || (!a.cfg.AllowAnonymous && strings.TrimSpace(a.cfg.APIKey) == "") {
		return nil, core.ErrProviderNotConfigured
	}
	path := ""
	var err error
	if a.cfg.PathBuilder != nil {
		path, err = a.cfg.PathBuilder(attempt)
	} else {
		path = a.cfg.Paths[attempt.Request.Protocol]
	}
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("%w: %s does not support %s", core.ErrCapabilityMismatch, a.cfg.Name, attempt.Request.Protocol)
	}
	endpoint, err := joinURL(a.cfg.BaseURL, path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(attempt.Request.Body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	if strings.TrimSpace(a.cfg.APIKey) != "" {
		req.Header.Set(a.cfg.AuthHeader, a.cfg.AuthPrefix+a.cfg.APIKey)
	}
	for k, v := range a.cfg.Headers {
		req.Header.Set(k, v)
	}
	for k, v := range attempt.Request.Headers {
		req.Header.Set(k, v)
	}
	resp, err := a.cfg.Client.Do(req)
	if err != nil {
		return nil, provider.WrapTransportError(a.cfg.Name, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, provider.NewHTTPError(a.cfg.Name, resp.StatusCode, string(raw), resp.Header, time.Now().UTC())
	}
	return newResponseStream(resp, a.cfg.Name, attempt.Request.Protocol, a.cfg.MaximumResponseBytes), nil
}
func (a *Adapter) ParseUsage(ctx context.Context, result provider.AttemptResult) (stream.Usage, error) {
	if result.Body == nil {
		return stream.Usage{}, core.ErrInvalidArgument
	}
	raw, err := readLimited(result.Body, a.cfg.MaximumResponseBytes)
	if err != nil {
		return stream.Usage{}, err
	}
	if err := ctx.Err(); err != nil {
		return stream.Usage{}, err
	}
	return ExtractUsage(raw), nil
}
func joinURL(base, path string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path, nil
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	return u.String(), nil
}

func readLimited(reader io.Reader, maximum int64) ([]byte, error) {
	if maximum <= 0 {
		return nil, fmt.Errorf("%w: response size limit", core.ErrInvalidConfiguration)
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximum {
		return nil, core.ErrFrameTooLarge
	}
	return raw, nil
}

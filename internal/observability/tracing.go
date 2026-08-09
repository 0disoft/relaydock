package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

type Shutdown func(context.Context) error

type Span struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Service      string
	Name         string
	StartedAt    time.Time
	Attributes   map[string]string
}

type SpanResult struct {
	Span
	EndedAt time.Time
	Error   string
}

type spanKey struct{}

type Exporter interface {
	Export(context.Context, SpanResult) error
	Shutdown(context.Context) error
}

var configuredService atomic.Value
var configuredExporter atomic.Value

func ConfigureTracing(_ context.Context, serviceName string) (Shutdown, error) {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return nil, fmt.Errorf("%w: tracing service name", core.ErrInvalidArgument)
	}
	configuredService.Store(serviceName)
	configuredExporter.Store(Exporter(discardExporter{}))
	return func(ctx context.Context) error {
		if exporter, ok := currentExporter(); ok {
			return exporter.Shutdown(ctx)
		}
		return nil
	}, nil
}

func ConfigureTracingWithExporter(_ context.Context, serviceName string, exporter Exporter) (Shutdown, error) {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" || exporter == nil {
		return nil, fmt.Errorf("%w: tracing configuration", core.ErrInvalidArgument)
	}
	configuredService.Store(serviceName)
	configuredExporter.Store(exporter)
	return exporter.Shutdown, nil
}

func StartSpan(ctx context.Context, name string, attributes map[string]string) (context.Context, func(error)) {
	service, _ := configuredService.Load().(string)
	if service == "" {
		service = "unconfigured"
	}
	parent, _ := ctx.Value(spanKey{}).(Span)
	traceID := parent.TraceID
	if traceID == "" {
		traceID = randomHex(16)
	}
	span := Span{
		TraceID:      traceID,
		SpanID:       randomHex(8),
		ParentSpanID: parent.SpanID,
		Service:      service,
		Name:         strings.TrimSpace(name),
		StartedAt:    time.Now().UTC(),
		Attributes:   cloneAttributes(attributes),
	}
	spanCtx := context.WithValue(ctx, spanKey{}, span)
	var ended atomic.Bool
	return spanCtx, func(spanErr error) {
		if !ended.CompareAndSwap(false, true) {
			return
		}
		result := SpanResult{Span: span, EndedAt: time.Now().UTC()}
		if spanErr != nil {
			result.Error = spanErr.Error()
		}
		if exporter, ok := currentExporter(); ok {
			_ = exporter.Export(context.Background(), result)
		}
	}
}

func SpanFromContext(ctx context.Context) (Span, bool) {
	span, ok := ctx.Value(spanKey{}).(Span)
	return span, ok
}

func currentExporter() (Exporter, bool) {
	value := configuredExporter.Load()
	if value == nil {
		return nil, false
	}
	exporter, ok := value.(Exporter)
	return exporter, ok
}

func randomHex(bytes int) string {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		now := time.Now().UTC().UnixNano()
		return fmt.Sprintf("%0*x", bytes*2, now)
	}
	return hex.EncodeToString(buffer)
}

func cloneAttributes(attributes map[string]string) map[string]string {
	if len(attributes) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(attributes))
	for key, value := range attributes {
		cloned[key] = value
	}
	return cloned
}

type discardExporter struct{}

func (discardExporter) Export(context.Context, SpanResult) error { return nil }
func (discardExporter) Shutdown(context.Context) error           { return nil }

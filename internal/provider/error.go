package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

// ErrorClass is deliberately provider-neutral. Routing and retry policy must
// not depend on provider-specific error strings.
type ErrorClass string

const (
	ErrorClassInvalidRequest ErrorClass = "invalid_request"
	ErrorClassAuthentication ErrorClass = "authentication"
	ErrorClassPermission     ErrorClass = "permission"
	ErrorClassNotFound       ErrorClass = "not_found"
	ErrorClassConflict       ErrorClass = "conflict"
	ErrorClassRateLimit      ErrorClass = "rate_limit"
	ErrorClassQuota          ErrorClass = "quota_exhausted"
	ErrorClassOverloaded     ErrorClass = "overloaded"
	ErrorClassTimeout        ErrorClass = "timeout"
	ErrorClassTransport      ErrorClass = "transport"
	ErrorClassUpstream       ErrorClass = "upstream"
	ErrorClassCancelled      ErrorClass = "cancelled"
	ErrorClassUnknown        ErrorClass = "unknown"
)

// UpstreamError retains enough structured information for retry, cooldown,
// metrics, and operator diagnostics without exposing credentials or an
// unbounded provider response body.
type UpstreamError struct {
	Provider   string
	Class      ErrorClass
	StatusCode int
	Message    string
	RetryAfter time.Duration
	Cause      error
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "provider error"
	}
	providerName := strings.TrimSpace(e.Provider)
	if providerName == "" {
		providerName = "provider"
	}
	parts := []string{providerName, string(e.Class)}
	if e.StatusCode > 0 {
		parts = append(parts, strconv.Itoa(e.StatusCode))
	}
	prefix := strings.Join(parts, ": ")
	message := strings.TrimSpace(e.Message)
	if message == "" && e.Cause != nil {
		message = e.Cause.Error()
	}
	if message == "" {
		return prefix
	}
	return prefix + ": " + message
}

func (e *UpstreamError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *UpstreamError) Retryable() bool {
	if e == nil {
		return false
	}
	switch e.Class {
	case ErrorClassRateLimit, ErrorClassOverloaded, ErrorClassTimeout, ErrorClassTransport, ErrorClassUpstream:
		return true
	default:
		return false
	}
}

// NewHTTPError converts an HTTP status and bounded provider response into the
// common error taxonomy. The body must already have been truncated by the
// caller.
func NewHTTPError(providerName string, statusCode int, body string, header http.Header, now time.Time) *UpstreamError {
	class := classifyHTTPStatus(statusCode, body)
	return &UpstreamError{
		Provider:   strings.TrimSpace(providerName),
		Class:      class,
		StatusCode: statusCode,
		Message:    sanitizeProviderMessage(body),
		RetryAfter: ParseRetryAfter(header.Get("Retry-After"), now),
	}
}

func WrapTransportError(providerName string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return &UpstreamError{Provider: providerName, Class: ErrorClassCancelled, Message: "request cancelled", Cause: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &UpstreamError{Provider: providerName, Class: ErrorClassTimeout, Message: "request deadline exceeded", Cause: err}
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return &UpstreamError{Provider: providerName, Class: ErrorClassTimeout, Message: "network timeout", Cause: err}
	}
	return &UpstreamError{Provider: providerName, Class: ErrorClassTransport, Message: "transport failure", Cause: err}
}

func IsRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, core.ErrRateLimited) {
		return true
	}
	var upstream *UpstreamError
	if errors.As(err, &upstream) {
		return upstream.Retryable()
	}
	var networkError net.Error
	return errors.As(err, &networkError) && (networkError.Timeout() || networkError.Temporary())
}

func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var upstream *UpstreamError
	if errors.As(err, &upstream) {
		return string(upstream.Class)
	}
	switch {
	case errors.Is(err, context.Canceled):
		return string(ErrorClassCancelled)
	case errors.Is(err, context.DeadlineExceeded):
		return string(ErrorClassTimeout)
	case errors.Is(err, core.ErrRateLimited):
		return string(ErrorClassRateLimit)
	case errors.Is(err, core.ErrUnauthorized):
		return string(ErrorClassAuthentication)
	case errors.Is(err, core.ErrForbidden):
		return string(ErrorClassPermission)
	case errors.Is(err, core.ErrNotFound):
		return string(ErrorClassNotFound)
	case errors.Is(err, core.ErrConflict):
		return string(ErrorClassConflict)
	default:
		return string(ErrorClassUnknown)
	}
}

func RetryAfter(err error) time.Duration {
	var upstream *UpstreamError
	if errors.As(err, &upstream) && upstream.RetryAfter > 0 {
		return upstream.RetryAfter
	}
	return 0
}

func ParseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	delay := when.Sub(now)
	if delay < 0 {
		return 0
	}
	return delay
}

func classifyHTTPStatus(statusCode int, body string) ErrorClass {
	lower := strings.ToLower(body)
	switch {
	case statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity:
		return ErrorClassInvalidRequest
	case statusCode == http.StatusUnauthorized:
		return ErrorClassAuthentication
	case statusCode == http.StatusForbidden:
		if strings.Contains(lower, "quota") || strings.Contains(lower, "credit") || strings.Contains(lower, "billing") {
			return ErrorClassQuota
		}
		return ErrorClassPermission
	case statusCode == http.StatusNotFound:
		return ErrorClassNotFound
	case statusCode == http.StatusConflict:
		return ErrorClassConflict
	case statusCode == http.StatusRequestTimeout || statusCode == http.StatusGatewayTimeout:
		return ErrorClassTimeout
	case statusCode == http.StatusTooManyRequests:
		if strings.Contains(lower, "quota") || strings.Contains(lower, "insufficient") || strings.Contains(lower, "credit") {
			return ErrorClassQuota
		}
		return ErrorClassRateLimit
	case statusCode == http.StatusServiceUnavailable || statusCode == http.StatusBadGateway || statusCode == 529:
		return ErrorClassOverloaded
	case statusCode >= 500:
		return ErrorClassUpstream
	default:
		return ErrorClassUnknown
	}
}

func sanitizeProviderMessage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return http.StatusText(http.StatusBadGateway)
	}
	value = strings.ReplaceAll(value, "\x00", "")
	const maximum = 2048
	if len(value) > maximum {
		value = value[:maximum] + "…"
	}
	return value
}

func (c ErrorClass) Validate() error {
	switch c {
	case ErrorClassInvalidRequest, ErrorClassAuthentication, ErrorClassPermission, ErrorClassNotFound,
		ErrorClassConflict, ErrorClassRateLimit, ErrorClassQuota, ErrorClassOverloaded,
		ErrorClassTimeout, ErrorClassTransport, ErrorClassUpstream, ErrorClassCancelled, ErrorClassUnknown:
		return nil
	default:
		return fmt.Errorf("%w: provider error class %q", core.ErrInvalidArgument, c)
	}
}

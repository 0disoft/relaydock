package apiutil

/* llmnav/1 module
id=relaydock.transport.json-api
role=Enforce bounded single-value JSON requests and map RelayDock domain errors into no-store JSON responses.
owns=HTTP JSON decoding boundary|HTTP error status mapping|JSON response headers
excludes=route authentication|domain error creation
search=read bounded JSON|map API error status|reject trailing JSON
invariant=Request decoding rejects unknown fields, oversized bodies, and more than one JSON value.
invariant=Every JSON response is marked no-store.
stability=contract
*/

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/0disoft/relaydock/internal/core"
)

const DefaultMaximumBodyBytes int64 = 8 << 20

type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func ReadJSON(w http.ResponseWriter, r *http.Request, limit int64, destination any) error {
	if limit <= 0 {
		limit = DefaultMaximumBodyBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("%w: malformed JSON: %v", core.ErrInvalidArgument, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("%w: more than one JSON value", core.ErrInvalidArgument)
		}
		return fmt.Errorf("%w: malformed trailing JSON: %v", core.ErrInvalidArgument, err)
	}
	return nil
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func WriteError(w http.ResponseWriter, err error) {
	status, code := classify(err)
	WriteJSON(w, status, ErrorBody{Error: ErrorDetail{Code: code, Message: err.Error()}})
}

func classify(err error) (int, string) {
	switch {
	case errors.Is(err, core.ErrInvalidArgument), errors.Is(err, core.ErrLossyTransformation), errors.Is(err, core.ErrCapabilityMismatch):
		return http.StatusBadRequest, "invalid_request"
	case errors.Is(err, core.ErrUnauthorized):
		return http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, core.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, core.ErrNotFound), errors.Is(err, core.ErrNoRoute):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, core.ErrConflict), errors.Is(err, core.ErrInvalidTransition):
		return http.StatusConflict, "conflict"
	case errors.Is(err, core.ErrFrameTooLarge):
		return http.StatusRequestEntityTooLarge, "payload_too_large"
	case errors.Is(err, core.ErrBudgetExceeded):
		return http.StatusPaymentRequired, "budget_exceeded"
	case errors.Is(err, core.ErrRateLimited):
		return http.StatusTooManyRequests, "rate_limited"
	case errors.Is(err, core.ErrInvalidConfiguration), errors.Is(err, core.ErrProviderNotConfigured):
		return http.StatusServiceUnavailable, "not_ready"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}

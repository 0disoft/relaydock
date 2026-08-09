package httpgateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
	"github.com/your-org/ai-runtime-gateway/internal/transport/apiutil"
)

func readBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON: %v", core.ErrInvalidArgument, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("%w: multiple JSON values", core.ErrInvalidArgument)
		}
		return nil, fmt.Errorf("%w: malformed trailing JSON: %v", core.ErrInvalidArgument, err)
	}
	return json.Marshal(value)
}

func requestMetadata(maximumBodyBytes int64, mode string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-AI-Runtime-Mode", mode)
		if r.Header.Get("X-Request-ID") == "" {
			r.Header.Set("X-Request-ID", idgen.New("http"))
		}
		w.Header().Set("X-Request-ID", r.Header.Get("X-Request-ID"))
		if raw := r.Header.Get("Content-Length"); raw != "" {
			if size, err := strconv.ParseInt(raw, 10, 64); err == nil && size > maximumBodyBytes {
				apiutil.WriteError(w, fmt.Errorf("%w: request body too large", core.ErrFrameTooLarge))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

package health

import (
	"encoding/json"
	"net/http"
)

type Handler struct {
	Ready func() bool
}

func (h Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h Handler) Readiness(w http.ResponseWriter, _ *http.Request) {
	ready := h.Ready == nil || h.Ready()
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{"ready": ready})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

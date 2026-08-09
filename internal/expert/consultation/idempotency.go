package consultation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type idempotencyPayload struct {
	TenantID         string `json:"tenantId"`
	ProjectID        string `json:"projectId"`
	Objective        string `json:"objective"`
	TaskType         string `json:"taskType"`
	Route            Route  `json:"route"`
	ContextPackID    string `json:"contextPackId"`
	MaximumCostMinor int64  `json:"maximumCostMinor"`
	TTLNanoseconds   int64  `json:"ttlNanoseconds"`
}

// RequestFingerprint prevents an idempotency key from silently returning a
// consultation created for different input.
func RequestFingerprint(command CreateCommand) string {
	payload := idempotencyPayload{
		TenantID:         strings.TrimSpace(command.TenantID),
		ProjectID:        strings.TrimSpace(command.ProjectID),
		Objective:        strings.TrimSpace(command.Objective),
		TaskType:         strings.TrimSpace(command.TaskType),
		Route:            command.Route,
		ContextPackID:    strings.TrimSpace(command.ContextPackID),
		MaximumCostMinor: command.MaximumCostMinor,
		TTLNanoseconds:   int64(command.TTL),
	}
	raw, _ := json.Marshal(payload)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func IdempotencyScope(command CreateCommand) string {
	key := strings.TrimSpace(command.IdempotencyKey)
	if key == "" {
		return ""
	}
	return strings.TrimSpace(command.TenantID) + "\x00" + strings.TrimSpace(command.ProjectID) + "\x00" + key
}

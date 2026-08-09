package resultcontract

import (
	"context"
	"strings"
	"time"
)

const UnverifiedAttestation = "unverified"

type Record struct {
	Result           Result    `json:"result"`
	ModelAttestation string    `json:"modelAttestation"`
	CreatedAt        time.Time `json:"createdAt"`
}

type RecordStore interface {
	PutRecord(context.Context, Record) (string, error)
	GetRecord(context.Context, string) (Record, error)
}

func NormalizeRecord(record Record) Record {
	record.ModelAttestation = strings.TrimSpace(record.ModelAttestation)
	if record.ModelAttestation == "" {
		record.ModelAttestation = UnverifiedAttestation
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	} else {
		record.CreatedAt = record.CreatedAt.UTC()
	}
	return record
}

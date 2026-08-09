package localstore

import (
	"sync"
	"time"

	"github.com/0disoft/relaydock/internal/expert/consultation"
	"github.com/0disoft/relaydock/internal/expert/contextpack"
	"github.com/0disoft/relaydock/internal/expert/resultcontract"
)

const (
	currentVersion      = 3
	maximumStateBytes   = int64(128 << 20)
	defaultContentChunk = 32 << 10
	chunkDigestPrefix   = "sha256:"
)

type idempotencyRecord struct {
	ConsultationID string `json:"consultationId"`
	Fingerprint    string `json:"fingerprint"`
}

type resultMetadata struct {
	ModelAttestation string    `json:"modelAttestation"`
	CreatedAt        time.Time `json:"createdAt"`
}

type chunkReference struct {
	Digest string `json:"digest"`
	Size   int    `json:"size"`
}

type evidenceContent struct {
	EvidenceIndex int              `json:"evidenceIndex"`
	Digest        string           `json:"digest"`
	Bytes         int64            `json:"bytes"`
	Chunks        []chunkReference `json:"chunks"`
}

type storedPack struct {
	Manifest contextpack.Pack  `json:"manifest"`
	Contents []evidenceContent `json:"contents,omitempty"`
}

type state struct {
	Version        int                                  `json:"version"`
	Consultations  map[string]consultation.Consultation `json:"consultations"`
	Idempotency    map[string]idempotencyRecord         `json:"idempotency"`
	ContextPacks   map[string]storedPack                `json:"contextPacks"`
	Results        map[string]resultcontract.Result     `json:"results"`
	ResultMetadata map[string]resultMetadata            `json:"resultMetadata,omitempty"`
}

type legacyState struct {
	Version        int                                  `json:"version"`
	Consultations  map[string]consultation.Consultation `json:"consultations"`
	Idempotency    map[string]idempotencyRecord         `json:"idempotency"`
	ContextPacks   map[string]contextpack.Pack          `json:"contextPacks"`
	Results        map[string]resultcontract.Result     `json:"results"`
	ResultMetadata map[string]resultMetadata            `json:"resultMetadata,omitempty"`
}

// Store is the durable single-user repository used by desktop and headless
// runtimes. Metadata mutations replace one JSON file atomically. ContextPack
// payloads are held in immutable, content-addressed chunks so large source
// excerpts do not rewrite the entire metadata file on every state transition.
type Store struct {
	mu      sync.RWMutex
	chunkMu sync.RWMutex
	path    string
	now     func() time.Time
	chunks  chunkStore
	data    state
}

func (s *Store) Path() string { return s.path }

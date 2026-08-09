package localstore

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/expert/consultation"
	"github.com/0disoft/relaydock/internal/expert/contextpack"
	"github.com/0disoft/relaydock/internal/expert/resultcontract"
)

func emptyState() state {
	return state{
		Version:        currentVersion,
		Consultations:  make(map[string]consultation.Consultation),
		Idempotency:    make(map[string]idempotencyRecord),
		ContextPacks:   make(map[string]storedPack),
		Results:        make(map[string]resultcontract.Result),
		ResultMetadata: make(map[string]resultMetadata),
	}
}

func normalizeAndValidate(value *state) error {
	if value.Version != currentVersion {
		return fmt.Errorf("%w: unsupported local expert store version %d", core.ErrInvalidConfiguration, value.Version)
	}
	if value.Consultations == nil {
		value.Consultations = make(map[string]consultation.Consultation)
	}
	if value.Idempotency == nil {
		value.Idempotency = make(map[string]idempotencyRecord)
	}
	if value.ContextPacks == nil {
		value.ContextPacks = make(map[string]storedPack)
	}
	if value.Results == nil {
		value.Results = make(map[string]resultcontract.Result)
	}
	if value.ResultMetadata == nil {
		value.ResultMetadata = make(map[string]resultMetadata)
	}
	if err := validateResults(value); err != nil {
		return err
	}
	if err := validatePacks(value); err != nil {
		return err
	}
	if err := validateConsultations(value); err != nil {
		return err
	}
	for _, record := range value.Idempotency {
		if _, exists := value.Consultations[record.ConsultationID]; !exists {
			return fmt.Errorf("%w: idempotency entry references missing consultation", core.ErrInvalidConfiguration)
		}
		if strings.TrimSpace(record.Fingerprint) == "" {
			return fmt.Errorf("%w: idempotency entry has no fingerprint", core.ErrInvalidConfiguration)
		}
	}
	return nil
}

func normalizeLegacy(value *legacyState) error {
	if value.Version == 0 {
		value.Version = 1
	}
	if value.Version < 1 || value.Version > 2 {
		return fmt.Errorf("%w: unsupported legacy local expert store version %d", core.ErrInvalidConfiguration, value.Version)
	}
	if value.Consultations == nil {
		value.Consultations = make(map[string]consultation.Consultation)
	}
	if value.Idempotency == nil {
		value.Idempotency = make(map[string]idempotencyRecord)
	}
	if value.ContextPacks == nil {
		value.ContextPacks = make(map[string]contextpack.Pack)
	}
	if value.Results == nil {
		value.Results = make(map[string]resultcontract.Result)
	}
	if value.ResultMetadata == nil {
		value.ResultMetadata = make(map[string]resultMetadata)
	}
	if value.Version == 1 {
		for id := range value.Results {
			value.ResultMetadata[id] = resultMetadata{ModelAttestation: resultcontract.UnverifiedAttestation}
		}
	}
	for id, item := range value.Consultations {
		if id == "" || item.ID != id {
			return fmt.Errorf("%w: malformed consultation entry", core.ErrInvalidConfiguration)
		}
		if item.ContextPackID != "" {
			if _, exists := value.ContextPacks[item.ContextPackID]; !exists {
				return fmt.Errorf("%w: consultation %s references missing context pack", core.ErrInvalidConfiguration, id)
			}
		}
		if item.ResultID != "" {
			if _, exists := value.Results[item.ResultID]; !exists {
				return fmt.Errorf("%w: consultation %s references missing result", core.ErrInvalidConfiguration, id)
			}
		}
	}
	for id := range value.Results {
		metadata, exists := value.ResultMetadata[id]
		if !exists {
			return fmt.Errorf("%w: result %s has no metadata", core.ErrInvalidConfiguration, id)
		}
		if strings.TrimSpace(metadata.ModelAttestation) == "" {
			metadata.ModelAttestation = resultcontract.UnverifiedAttestation
			value.ResultMetadata[id] = metadata
		}
	}
	for id := range value.ResultMetadata {
		if _, exists := value.Results[id]; !exists {
			return fmt.Errorf("%w: result metadata references missing result", core.ErrInvalidConfiguration)
		}
	}
	for _, record := range value.Idempotency {
		if _, exists := value.Consultations[record.ConsultationID]; !exists {
			return fmt.Errorf("%w: idempotency entry references missing consultation", core.ErrInvalidConfiguration)
		}
	}
	return nil
}

func validateResults(value *state) error {
	for id := range value.Results {
		metadata, exists := value.ResultMetadata[id]
		if !exists {
			return fmt.Errorf("%w: result %s has no metadata", core.ErrInvalidConfiguration, id)
		}
		if strings.TrimSpace(metadata.ModelAttestation) == "" {
			metadata.ModelAttestation = resultcontract.UnverifiedAttestation
			value.ResultMetadata[id] = metadata
		}
	}
	for id := range value.ResultMetadata {
		if _, exists := value.Results[id]; !exists {
			return fmt.Errorf("%w: result metadata references missing result", core.ErrInvalidConfiguration)
		}
	}
	return nil
}

func validatePacks(value *state) error {
	for id, stored := range value.ContextPacks {
		if id == "" || stored.Manifest.ID != id {
			return fmt.Errorf("%w: malformed context pack entry", core.ErrInvalidConfiguration)
		}
		seenEvidence := make(map[int]struct{}, len(stored.Contents))
		for _, content := range stored.Contents {
			if content.EvidenceIndex < 0 || content.EvidenceIndex >= len(stored.Manifest.Evidence) {
				return fmt.Errorf("%w: context pack %s has invalid evidence index", core.ErrInvalidConfiguration, id)
			}
			if _, duplicate := seenEvidence[content.EvidenceIndex]; duplicate {
				return fmt.Errorf("%w: context pack %s has duplicate evidence content", core.ErrInvalidConfiguration, id)
			}
			seenEvidence[content.EvidenceIndex] = struct{}{}
			if content.Bytes <= 0 || strings.TrimSpace(content.Digest) == "" || len(content.Chunks) == 0 {
				return fmt.Errorf("%w: context pack %s has malformed evidence content", core.ErrInvalidConfiguration, id)
			}
			if stored.Manifest.Evidence[content.EvidenceIndex].Content != "" {
				return fmt.Errorf("%w: context pack %s embeds chunked evidence content", core.ErrInvalidConfiguration, id)
			}
		}
	}
	return nil
}

func validateConsultations(value *state) error {
	for id, item := range value.Consultations {
		if id == "" || item.ID != id {
			return fmt.Errorf("%w: malformed consultation entry", core.ErrInvalidConfiguration)
		}
		if item.ContextPackID != "" {
			if _, exists := value.ContextPacks[item.ContextPackID]; !exists {
				return fmt.Errorf("%w: consultation %s references missing context pack", core.ErrInvalidConfiguration, id)
			}
		}
		if item.ResultID != "" {
			if _, exists := value.Results[item.ResultID]; !exists {
				return fmt.Errorf("%w: consultation %s references missing result", core.ErrInvalidConfiguration, id)
			}
		}
	}
	return nil
}

func cloneState(value state) (state, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return state{}, err
	}
	var cloned state
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return state{}, err
	}
	return cloned, normalizeAndValidate(&cloned)
}

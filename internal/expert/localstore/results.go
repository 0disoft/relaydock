package localstore

import (
	"context"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/expert/resultcontract"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
)

func (s *Store) CommitClaimResult(ctx context.Context, consultationID, workerID string, result resultcontract.Result) (consultation.Consultation, string, error) {
	return s.CommitClaimResultAttested(ctx, consultationID, workerID, resultcontract.UnverifiedAttestation, result)
}

func (s *Store) CommitClaimResultAttested(ctx context.Context, consultationID, workerID, modelAttestation string, result resultcontract.Result) (consultation.Consultation, string, error) {
	record := resultcontract.NormalizeRecord(resultcontract.Record{Result: result, ModelAttestation: modelAttestation})
	if err := resultcontract.Validate(record.Result); err != nil {
		return consultation.Consultation{}, "", err
	}
	resultID := idgen.New("result")
	var updated consultation.Consultation
	err := s.update(ctx, func(next *state) error {
		item, exists := next.Consultations[consultationID]
		if !exists {
			return core.ErrNotFound
		}
		if err := consultation.ValidateClaimOwner(item, workerID, s.nowUTC()); err != nil {
			return err
		}
		storeResult(next, resultID, record)
		completeConsultation(&item, resultID, s.nowUTC())
		next.Consultations[item.ID] = item
		updated = item
		return nil
	})
	if err != nil {
		return consultation.Consultation{}, "", err
	}
	return cloneConsultation(updated), resultID, nil
}

func (s *Store) CommitResult(ctx context.Context, consultationID string, result resultcontract.Result) (consultation.Consultation, string, error) {
	return s.CommitResultAttested(ctx, consultationID, resultcontract.UnverifiedAttestation, result)
}

func (s *Store) CommitResultAttested(ctx context.Context, consultationID, modelAttestation string, result resultcontract.Result) (consultation.Consultation, string, error) {
	record := resultcontract.NormalizeRecord(resultcontract.Record{Result: result, ModelAttestation: modelAttestation})
	if err := resultcontract.Validate(record.Result); err != nil {
		return consultation.Consultation{}, "", err
	}
	resultID := idgen.New("result")
	var updated consultation.Consultation
	err := s.update(ctx, func(next *state) error {
		item, exists := next.Consultations[consultationID]
		if !exists {
			return core.ErrNotFound
		}
		if item.State == consultation.StateQueued {
			item.State = consultation.StateRunning
			item.UpdatedAt = s.nowUTC()
		}
		if item.State != consultation.StateRunning && item.State != consultation.StateResultPending {
			return core.ErrConflict
		}
		storeResult(next, resultID, record)
		completeConsultation(&item, resultID, s.nowUTC())
		next.Consultations[item.ID] = item
		updated = item
		return nil
	})
	if err != nil {
		return consultation.Consultation{}, "", err
	}
	return cloneConsultation(updated), resultID, nil
}

func (s *Store) PutResult(ctx context.Context, result resultcontract.Result) (string, error) {
	return s.PutResultRecord(ctx, resultcontract.Record{Result: result, ModelAttestation: resultcontract.UnverifiedAttestation})
}

func (s *Store) PutResultRecord(ctx context.Context, record resultcontract.Record) (string, error) {
	record = resultcontract.NormalizeRecord(record)
	if err := resultcontract.Validate(record.Result); err != nil {
		return "", err
	}
	id := idgen.New("result")
	err := s.update(ctx, func(next *state) error { storeResult(next, id, record); return nil })
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) GetResult(ctx context.Context, id string) (resultcontract.Result, error) {
	record, err := s.GetResultRecord(ctx, id)
	return record.Result, err
}

func (s *Store) GetResultRecord(ctx context.Context, id string) (resultcontract.Record, error) {
	if err := ctx.Err(); err != nil {
		return resultcontract.Record{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return resultcontract.Record{}, core.ErrInvalidArgument
	}
	s.mu.RLock()
	result, resultExists := s.data.Results[id]
	metadata, metadataExists := s.data.ResultMetadata[id]
	s.mu.RUnlock()
	if !resultExists || !metadataExists {
		return resultcontract.Record{}, core.ErrNotFound
	}
	return resultcontract.NormalizeRecord(resultcontract.Record{
		Result: cloneResult(result), ModelAttestation: metadata.ModelAttestation, CreatedAt: metadata.CreatedAt,
	}), nil
}

func storeResult(next *state, id string, record resultcontract.Record) {
	next.Results[id] = cloneResult(record.Result)
	next.ResultMetadata[id] = resultMetadata{ModelAttestation: record.ModelAttestation, CreatedAt: record.CreatedAt}
}

func completeConsultation(item *consultation.Consultation, resultID string, now time.Time) {
	item.ResultID = resultID
	item.State = consultation.StateCompleted
	item.UpdatedAt = now
	consultation.ClearClaim(item)
}

package postgresstore

import (
	"database/sql"
	"fmt"

	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
)

type rowScanner interface {
	Scan(...any) error
}

const consultationColumns = `
    id,
    tenant_id,
    project_id::text,
    objective,
    task_type,
    route,
    state,
    context_pack_id,
    maximum_cost_minor,
    COALESCE(result_id, ''),
    COALESCE(failure_reason, ''),
    attempt_count,
    available_at,
    COALESCE(locked_by, ''),
    lock_expires_at,
    created_at,
    updated_at,
    expires_at`

func scanConsultation(row rowScanner) (consultation.Consultation, error) {
	var item consultation.Consultation
	var route string
	var state string
	var lockExpires sql.NullTime
	if err := row.Scan(
		&item.ID,
		&item.TenantID,
		&item.ProjectID,
		&item.Objective,
		&item.TaskType,
		&route,
		&state,
		&item.ContextPackID,
		&item.MaximumCostMinor,
		&item.ResultID,
		&item.FailureReason,
		&item.AttemptCount,
		&item.AvailableAt,
		&item.LockedBy,
		&lockExpires,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.ExpiresAt,
	); err != nil {
		return consultation.Consultation{}, err
	}
	item.Route = consultation.Route(route)
	item.State = consultation.State(state)
	if lockExpires.Valid {
		item.LockExpiresAt = lockExpires.Time.UTC()
	}
	item.AvailableAt = item.AvailableAt.UTC()
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	item.ExpiresAt = item.ExpiresAt.UTC()
	if item.ID == "" {
		return consultation.Consultation{}, fmt.Errorf("empty consultation ID returned by database")
	}
	return item, nil
}

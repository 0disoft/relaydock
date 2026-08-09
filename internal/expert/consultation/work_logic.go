package consultation

import (
	"fmt"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

const (
	defaultClaimLease  = 2 * time.Minute
	defaultMaxAttempts = 3
)

func NormalizeClaimCommand(command ClaimCommand) (ClaimCommand, error) {
	command.WorkerID = strings.TrimSpace(command.WorkerID)
	if command.WorkerID == "" {
		return ClaimCommand{}, fmt.Errorf("%w: worker ID", core.ErrInvalidArgument)
	}
	if command.LeaseTTL <= 0 {
		command.LeaseTTL = defaultClaimLease
	}
	if command.LeaseTTL < 5*time.Second || command.LeaseTTL > 30*time.Minute {
		return ClaimCommand{}, fmt.Errorf("%w: claim lease must be between 5 seconds and 30 minutes", core.ErrInvalidArgument)
	}
	if command.ClaimedAt.IsZero() {
		command.ClaimedAt = time.Now().UTC()
	} else {
		command.ClaimedAt = command.ClaimedAt.UTC()
	}
	if command.MaxAttempts <= 0 {
		command.MaxAttempts = defaultMaxAttempts
	}
	if command.MaxAttempts > 20 {
		return ClaimCommand{}, fmt.Errorf("%w: maximum attempts exceeds 20", core.ErrInvalidArgument)
	}
	return command, nil
}

func Claimable(item Consultation, command ClaimCommand) bool {
	if item.State != StateQueued || item.Terminal() {
		return false
	}
	if command.Route != "" && item.Route != command.Route {
		return false
	}
	if !item.AvailableAt.IsZero() && item.AvailableAt.After(command.ClaimedAt) {
		return false
	}
	return true
}

func ApplyClaim(item *Consultation, command ClaimCommand) error {
	if item == nil {
		return core.ErrInvalidArgument
	}
	normalized, err := NormalizeClaimCommand(command)
	if err != nil {
		return err
	}
	if !Claimable(*item, normalized) {
		return core.ErrConflict
	}
	if item.AttemptCount >= normalized.MaxAttempts {
		item.State = StateFailed
		item.FailureReason = "maximum execution attempts exceeded"
		item.UpdatedAt = normalized.ClaimedAt
		clearClaim(item)
		return core.ErrConflict
	}
	item.State = StateRunning
	item.AttemptCount++
	item.LockedBy = normalized.WorkerID
	item.LockExpiresAt = normalized.ClaimedAt.Add(normalized.LeaseTTL)
	item.AvailableAt = time.Time{}
	item.FailureReason = ""
	item.UpdatedAt = normalized.ClaimedAt
	return nil
}

func ApplyRenewClaim(item *Consultation, workerID string, ttl time.Duration, now time.Time) error {
	if item == nil {
		return core.ErrInvalidArgument
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || ttl < 5*time.Second || ttl > 30*time.Minute {
		return core.ErrInvalidArgument
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if item.State != StateRunning || item.LockedBy != workerID || (!item.LockExpiresAt.IsZero() && !item.LockExpiresAt.After(now)) {
		return core.ErrConflict
	}
	item.LockExpiresAt = now.Add(ttl)
	item.UpdatedAt = now
	return nil
}

func ValidateClaimOwner(item Consultation, workerID string, now time.Time) error {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return core.ErrInvalidArgument
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if item.State != StateRunning || item.LockedBy != workerID || item.LockExpiresAt.IsZero() || !item.LockExpiresAt.After(now) {
		return core.ErrLeaseLost
	}
	return nil
}

func ApplyRetryClaim(item *Consultation, workerID string, availableAt time.Time, reason string, maximumAttempts int, now time.Time) error {
	if item == nil {
		return core.ErrInvalidArgument
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || item.State != StateRunning || item.LockedBy != workerID {
		return core.ErrConflict
	}
	if maximumAttempts <= 0 {
		maximumAttempts = defaultMaxAttempts
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	item.FailureReason = boundedFailureReason(reason)
	item.UpdatedAt = now
	clearClaim(item)
	if item.AttemptCount >= maximumAttempts {
		item.State = StateFailed
		item.AvailableAt = time.Time{}
		return nil
	}
	if availableAt.Before(now) {
		availableAt = now
	}
	item.State = StateQueued
	item.AvailableAt = availableAt.UTC()
	return nil
}

func ApplyStaleRecovery(item *Consultation, now time.Time, maximumAttempts int) bool {
	if item == nil || item.State != StateRunning || item.LockExpiresAt.IsZero() || item.LockExpiresAt.After(now) {
		return false
	}
	if maximumAttempts <= 0 {
		maximumAttempts = defaultMaxAttempts
	}
	item.UpdatedAt = now.UTC()
	item.FailureReason = "worker lease expired before completion"
	clearClaim(item)
	if item.AttemptCount >= maximumAttempts {
		item.State = StateFailed
		item.AvailableAt = time.Time{}
	} else {
		item.State = StateQueued
		item.AvailableAt = now.UTC()
	}
	return true
}

func ClearClaim(item *Consultation) {
	clearClaim(item)
}

func clearClaim(item *Consultation) {
	item.LockedBy = ""
	item.LockExpiresAt = time.Time{}
}

func boundedFailureReason(value string) string {
	value = strings.TrimSpace(value)
	const maximum = 2048
	if len(value) > maximum {
		return value[:maximum] + "…"
	}
	return value
}

package snapshot

/* llmnav/1 module
id=relaydock.control.snapshot
role=Verify, apply, persist, and refresh signed runtime snapshots while preserving a last-known-good fallback.
owns=runtime snapshot lifecycle|signature and revision acceptance|last-known-good synchronization
excludes=control HTTP authorization|gateway route execution
search=signed runtime snapshot|last known good routes|reject snapshot rollback
invariant=An unsigned, expired, rolled-back, or content-mutated revision is never applied as current runtime state.
invariant=A last-known-good persistence failure cannot replace or invalidate an already verified active snapshot.
risk=auth|concurrency
rel=test>relaydock.control.snapshot.contract
stability=architecture
*/

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

type ApplyFunc func(context.Context, Snapshot) error

type SyncStatus struct {
	Revision          int64     `json:"revision"`
	ExpiresAt         time.Time `json:"expiresAt,omitempty"`
	LastSuccessAt     time.Time `json:"lastSuccessAt,omitempty"`
	LastAttemptAt     time.Time `json:"lastAttemptAt,omitempty"`
	LastError         string    `json:"lastError,omitempty"`
	UsingLastKnown    bool      `json:"usingLastKnownGood"`
	LastKnownRevision int64     `json:"lastKnownGoodRevision,omitempty"`
}

type Manager struct {
	Client       *RemoteClient
	LastKnown    Store
	Apply        ApplyFunc
	Required     bool
	FetchTimeout time.Duration
	BackoffMin   time.Duration
	BackoffMax   time.Duration
	Logger       *slog.Logger
	Now          func() time.Time

	mu      sync.RWMutex
	status  SyncStatus
	current Snapshot
	etag    string
}

func (m *Manager) Bootstrap(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	remoteErr := m.fetchAndApply(ctx)
	if remoteErr == nil || errors.Is(remoteErr, core.ErrNotModified) {
		return nil
	}
	lastKnownErr := m.applyLastKnown(ctx)
	if lastKnownErr == nil {
		m.setError(remoteErr, true)
		if m.Logger != nil {
			m.Logger.WarnContext(ctx, "control snapshot fetch failed; using last-known-good snapshot", "error", remoteErr)
		}
		return nil
	}
	combined := errors.Join(remoteErr, fmt.Errorf("load last-known-good runtime snapshot: %w", lastKnownErr))
	m.setError(combined, false)
	if m.Required {
		return combined
	}
	if m.Logger != nil {
		m.Logger.WarnContext(ctx, "control snapshot unavailable; keeping static routes", "error", combined)
	}
	return nil
}

// Run keeps the current snapshot fresh. It uses the watch stream for low
// latency and falls back to a conditional fetch after every disconnect. The
// caller should invoke Bootstrap first so startup policy is explicit.
func (m *Manager) Run(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	backoff := m.backoffMinimum()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		after := m.Status().Revision
		err := m.Client.Watch(ctx, after, func(applyCtx context.Context, value Snapshot) error {
			if err := m.applyVerified(applyCtx, value, false); err != nil {
				return err
			}
			backoff = m.backoffMinimum()
			return nil
		})
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			m.setError(err, m.Status().UsingLastKnown)
			if m.Logger != nil {
				m.Logger.WarnContext(ctx, "control snapshot watch disconnected", "error", err, "retryAfter", backoff)
			}
		}
		fetchErr := m.fetchAndApply(ctx)
		if fetchErr == nil || errors.Is(fetchErr, core.ErrNotModified) {
			backoff = m.backoffMinimum()
		} else {
			m.setError(fetchErr, m.Status().UsingLastKnown)
		}
		if err := sleepManager(ctx, backoff); err != nil {
			return err
		}
		backoff *= 2
		if backoff > m.backoffMaximum() {
			backoff = m.backoffMaximum()
		}
	}
}

func (m *Manager) Ready() bool {
	status := m.Status()
	return status.Revision > 0 && status.ExpiresAt.After(m.now())
}

func (m *Manager) Status() SyncStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Manager) Current() (Snapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.current.Revision <= 0 {
		return Snapshot{}, false
	}
	return cloneSnapshot(m.current), true
}

func (m *Manager) fetchAndApply(ctx context.Context) error {
	m.markAttempt()
	fetchCtx := ctx
	cancel := func() {}
	if timeout := m.fetchTimeout(); timeout > 0 {
		fetchCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	m.mu.RLock()
	etag := m.etag
	m.mu.RUnlock()
	value, nextETag, err := m.Client.FetchCurrent(fetchCtx, etag)
	if err != nil {
		return err
	}
	if err := m.applyVerified(ctx, value, false); err != nil {
		return err
	}
	m.mu.Lock()
	m.etag = strings.TrimSpace(nextETag)
	m.mu.Unlock()
	return nil
}

func (m *Manager) applyLastKnown(ctx context.Context) error {
	if m.LastKnown == nil {
		return core.ErrNotFound
	}
	value, err := m.LastKnown.Current(ctx)
	if err != nil {
		return err
	}
	return m.applyVerified(ctx, value, true)
}

func (m *Manager) applyVerified(ctx context.Context, value Snapshot, fromLastKnown bool) error {
	if err := m.Client.verify(value); err != nil {
		return err
	}

	m.mu.RLock()
	currentStatus := m.status
	currentSnapshot := cloneSnapshot(m.current)
	m.mu.RUnlock()
	if value.Revision < currentStatus.Revision {
		return fmt.Errorf("%w: runtime snapshot rollback from %d to %d", core.ErrConflict, currentStatus.Revision, value.Revision)
	}
	if value.Revision == currentStatus.Revision && currentStatus.Revision > 0 {
		equal, err := equivalentSnapshots(value, currentSnapshot)
		if err != nil {
			return err
		}
		if !equal {
			return fmt.Errorf("%w: runtime snapshot revision %d was reused with different content", core.ErrConflict, value.Revision)
		}
		lastKnownRevision, persistenceFailure, err := m.synchronizeLastKnown(ctx, value)
		if err != nil {
			return err
		}
		m.mu.Lock()
		m.status.LastSuccessAt = m.now()
		m.status.UsingLastKnown = fromLastKnown
		if lastKnownRevision > m.status.LastKnownRevision {
			m.status.LastKnownRevision = lastKnownRevision
		}
		if persistenceFailure != nil {
			m.status.LastError = persistenceFailure.Error()
		} else {
			m.status.LastError = ""
		}
		m.mu.Unlock()
		if persistenceFailure != nil && m.Logger != nil {
			m.Logger.ErrorContext(ctx, "runtime snapshot remains active but last-known-good persistence failed", "revision", value.Revision, "error", persistenceFailure)
		}
		return nil
	}

	persisted, persistedSet, persistedErr := m.readLastKnown(ctx)
	if persistedSet {
		if persisted.Revision > value.Revision {
			return fmt.Errorf("%w: remote runtime snapshot revision %d is older than last-known-good revision %d", core.ErrConflict, value.Revision, persisted.Revision)
		}
		if persisted.Revision == value.Revision {
			equal, err := equivalentSnapshots(value, persisted)
			if err != nil {
				return err
			}
			if !equal {
				return fmt.Errorf("%w: runtime snapshot revision %d conflicts with last-known-good content", core.ErrConflict, value.Revision)
			}
		}
	}

	if err := m.Apply(ctx, cloneSnapshot(value)); err != nil {
		return fmt.Errorf("apply runtime snapshot revision %d: %w", value.Revision, err)
	}

	now := m.now()
	m.mu.Lock()
	lastAttempt := m.status.LastAttemptAt
	lastKnownRevision := m.status.LastKnownRevision
	if persistedSet && persisted.Revision > lastKnownRevision {
		lastKnownRevision = persisted.Revision
	}
	m.current = cloneSnapshot(value)
	m.status = SyncStatus{
		Revision:          value.Revision,
		ExpiresAt:         value.ExpiresAt,
		LastSuccessAt:     now,
		LastAttemptAt:     lastAttempt,
		UsingLastKnown:    fromLastKnown,
		LastKnownRevision: lastKnownRevision,
	}
	m.mu.Unlock()

	var persistenceFailure error
	switch {
	case persistedErr != nil:
		persistenceFailure = fmt.Errorf("read last-known-good runtime snapshot: %w", persistedErr)
	case m.LastKnown != nil && (!persistedSet || value.Revision > persisted.Revision):
		if err := m.LastKnown.Publish(ctx, value); err != nil {
			persistenceFailure = fmt.Errorf("persist last-known-good runtime snapshot: %w", err)
		} else {
			m.mu.Lock()
			m.status.LastKnownRevision = value.Revision
			m.mu.Unlock()
		}
	}
	if persistenceFailure != nil {
		m.mu.Lock()
		m.status.LastError = persistenceFailure.Error()
		m.mu.Unlock()
		if m.Logger != nil {
			m.Logger.ErrorContext(ctx, "runtime snapshot is active but last-known-good persistence failed", "revision", value.Revision, "error", persistenceFailure)
		}
	}
	if m.Logger != nil {
		m.Logger.InfoContext(ctx, "runtime snapshot applied", "revision", value.Revision, "expiresAt", value.ExpiresAt, "lastKnownGood", fromLastKnown)
	}
	return nil
}

func (m *Manager) synchronizeLastKnown(ctx context.Context, value Snapshot) (int64, error, error) {
	persisted, set, readErr := m.readLastKnown(ctx)
	if readErr != nil {
		return m.Status().LastKnownRevision, fmt.Errorf("read last-known-good runtime snapshot: %w", readErr), nil
	}
	if set {
		if persisted.Revision > value.Revision {
			return persisted.Revision, nil, fmt.Errorf("%w: runtime snapshot revision %d is older than last-known-good revision %d", core.ErrConflict, value.Revision, persisted.Revision)
		}
		if persisted.Revision == value.Revision {
			equal, err := equivalentSnapshots(value, persisted)
			if err != nil {
				return persisted.Revision, nil, err
			}
			if !equal {
				return persisted.Revision, nil, fmt.Errorf("%w: runtime snapshot revision %d conflicts with last-known-good content", core.ErrConflict, value.Revision)
			}
			return persisted.Revision, nil, nil
		}
	}
	if m.LastKnown == nil {
		return 0, nil, nil
	}
	if err := m.LastKnown.Publish(ctx, value); err != nil {
		return m.Status().LastKnownRevision, fmt.Errorf("persist last-known-good runtime snapshot: %w", err), nil
	}
	return value.Revision, nil, nil
}

func (m *Manager) readLastKnown(ctx context.Context) (Snapshot, bool, error) {
	if m.LastKnown == nil {
		return Snapshot{}, false, nil
	}
	value, err := m.LastKnown.Current(ctx)
	if errors.Is(err, core.ErrNotFound) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, err
	}
	if err := m.Client.verify(value); err != nil {
		return Snapshot{}, false, fmt.Errorf("verify last-known-good runtime snapshot: %w", err)
	}
	return value, true, nil
}

func equivalentSnapshots(left, right Snapshot) (bool, error) {
	leftPayload, err := canonicalBytes(left)
	if err != nil {
		return false, fmt.Errorf("encode runtime snapshot: %w", err)
	}
	rightPayload, err := canonicalBytes(right)
	if err != nil {
		return false, fmt.Errorf("encode current runtime snapshot: %w", err)
	}
	return bytes.Equal(leftPayload, rightPayload), nil
}

func (m *Manager) setError(err error, usingLastKnown bool) {
	if err == nil {
		return
	}
	m.mu.Lock()
	m.status.LastError = err.Error()
	m.status.UsingLastKnown = usingLastKnown
	m.mu.Unlock()
}

func (m *Manager) markAttempt() {
	m.mu.Lock()
	m.status.LastAttemptAt = m.now()
	m.mu.Unlock()
}

func (m *Manager) validate() error {
	if m == nil || m.Client == nil || m.Apply == nil {
		return fmt.Errorf("%w: runtime snapshot manager", core.ErrInvalidConfiguration)
	}
	return nil
}

func (m *Manager) fetchTimeout() time.Duration {
	if m.FetchTimeout <= 0 {
		return 15 * time.Second
	}
	return m.FetchTimeout
}

func (m *Manager) backoffMinimum() time.Duration {
	if m.BackoffMin <= 0 {
		return time.Second
	}
	return m.BackoffMin
}

func (m *Manager) backoffMaximum() time.Duration {
	if m.BackoffMax <= 0 {
		return 30 * time.Second
	}
	return m.BackoffMax
}

func (m *Manager) now() time.Time {
	if m.Now == nil {
		return time.Now().UTC()
	}
	return m.Now().UTC()
}

func sleepManager(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

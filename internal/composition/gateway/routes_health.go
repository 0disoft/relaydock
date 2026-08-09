package gateway

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/provider"
	"github.com/0disoft/relaydock/internal/routing"
)

func (s *CandidateSource) RecordOutcome(ctx context.Context, outcome routing.Outcome) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := candidateRuntimeKey(outcome.Candidate)
	if key == "//" {
		return nil
	}
	now := s.currentTime()
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[key]
	if state.HealthMultiplier <= 0 {
		state.HealthMultiplier = 1
	}
	if outcome.Success {
		state.ConsecutiveFailures = 0
		state.CooldownUntil = time.Time{}
		state.HealthMultiplier += 0.15
		if state.HealthMultiplier > 1 {
			state.HealthMultiplier = 1
		}
		state.LastLatency = outcome.Latency()
		state.LastSeen = now
		s.states[key] = state
		s.pruneRuntimeStatesLocked(now)
		return nil
	}
	if outcome.ErrorCode == string(provider.ErrorClassCancelled) {
		return nil
	}
	state.ConsecutiveFailures++
	state.HealthMultiplier *= 0.7
	if state.HealthMultiplier < 0.1 {
		state.HealthMultiplier = 0.1
	}
	delay := cooldownForOutcome(outcome, state.ConsecutiveFailures)
	if delay > 0 {
		state.CooldownUntil = now.Add(delay)
	}
	state.LastLatency = outcome.Latency()
	state.LastSeen = now
	s.states[key] = state
	s.pruneRuntimeStatesLocked(now)
	return nil
}

func (s *CandidateSource) applyRuntimeState(candidates []routing.Candidate) []routing.Candidate {
	now := s.currentTime()
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range candidates {
		state, exists := s.states[candidateRuntimeKey(candidates[index])]
		if !exists {
			continue
		}
		multiplier := state.HealthMultiplier
		if multiplier <= 0 {
			multiplier = 1
		}
		candidates[index].HealthScore *= multiplier
		if state.CooldownUntil.After(now) {
			candidates[index].Available = false
			candidates[index].HealthScore = 0
		}
	}
	return candidates
}

func (s *CandidateSource) pruneRuntimeStatesLocked(now time.Time) {
	ttl := s.runtimeStateTTL
	if ttl <= 0 {
		ttl = defaultRuntimeStateTTL
	}
	cutoff := now.Add(-ttl)
	for key, state := range s.states {
		if !state.LastSeen.IsZero() && state.LastSeen.Before(cutoff) && !state.CooldownUntil.After(now) {
			delete(s.states, key)
		}
	}
	maximum := s.maximumRuntimeStates
	if maximum <= 0 {
		maximum = defaultMaximumRuntimeStates
	}
	if len(s.states) <= maximum {
		return
	}
	type stateAge struct {
		key      string
		lastSeen time.Time
	}
	ages := make([]stateAge, 0, len(s.states))
	for key, state := range s.states {
		ages = append(ages, stateAge{key: key, lastSeen: state.LastSeen})
	}
	sort.Slice(ages, func(i, j int) bool {
		if ages[i].lastSeen.Equal(ages[j].lastSeen) {
			return ages[i].key < ages[j].key
		}
		return ages[i].lastSeen.Before(ages[j].lastSeen)
	})
	for index := 0; index < len(ages)-maximum; index++ {
		delete(s.states, ages[index].key)
	}
}

func (s *CandidateSource) currentTime() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentTimeLocked()
}

func (s *CandidateSource) currentTimeLocked() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func cooldownForOutcome(outcome routing.Outcome, failures int) time.Duration {
	if outcome.RetryAfter > 0 {
		return clampDuration(outcome.RetryAfter, time.Second, time.Hour)
	}
	switch provider.ErrorClass(outcome.ErrorCode) {
	case provider.ErrorClassInvalidRequest, provider.ErrorClassConflict, provider.ErrorClassCancelled:
		return 0
	case provider.ErrorClassRateLimit:
		return exponentialCooldown(30*time.Second, failures, 10*time.Minute)
	case provider.ErrorClassQuota:
		return exponentialCooldown(5*time.Minute, failures, time.Hour)
	case provider.ErrorClassAuthentication, provider.ErrorClassPermission, provider.ErrorClassNotFound:
		return 10 * time.Minute
	case provider.ErrorClassOverloaded, provider.ErrorClassTimeout, provider.ErrorClassTransport, provider.ErrorClassUpstream:
		return exponentialCooldown(2*time.Second, failures, time.Minute)
	default:
		return exponentialCooldown(5*time.Second, failures, time.Minute)
	}
}

func exponentialCooldown(base time.Duration, failures int, maximum time.Duration) time.Duration {
	if failures < 1 {
		failures = 1
	}
	delay := base
	for index := 1; index < failures; index++ {
		if delay >= maximum/2 {
			return maximum
		}
		delay *= 2
	}
	return clampDuration(delay, base, maximum)
}

func clampDuration(value, minimum, maximum time.Duration) time.Duration {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func candidateRuntimeKey(candidate routing.Candidate) string {
	return strings.TrimSpace(candidate.Provider) + "/" + strings.TrimSpace(candidate.AccountID) + "/" + strings.TrimSpace(candidate.Model)
}
